// Package slcan drives CANable-style SLCAN adapters over their virtual COM
// port. Importing the package registers the "SLCan" adapter.
//
// Wire protocol (ASCII, CR terminated): Sn sets the bit-rate (S9 = the
// CANable custom 615.384 kbit/s), O/C open/close the channel and standard
// frames travel as "t iii l dd..". The firmware acks transmits with 'z';
// the ack is not gated on (fire-and-forget like v1).
package slcan

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:               "SLCan",
		Description:        "Canable SLCan adapter",
		RequiresSerialPort: true,
		Capabilities:       gocan.Capabilities{HSCAN: true},
		New:                New,
	})
}

type SLCan struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	port serial.Port
	line []byte
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &SLCan{cfg: cfg}, nil
}

func (sl *SLCan) Open(ctx context.Context, bus *gocan.Bus) error {
	sl.bus = bus
	p, err := serial.Open(sl.cfg.Port, &serial.Mode{
		BaudRate: sl.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		InitialStatusBits: &serial.ModemOutputBits{
			DTR: false,
			RTS: false,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to open com port %q: %w", sl.cfg.Port, err)
	}
	p.SetReadTimeout(3 * time.Millisecond)
	sl.port = p
	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	rate, err := bitRate(sl.cfg.CANRate)
	if err != nil {
		p.Close()
		return err
	}
	if _, err := p.Write([]byte(rate + "\r")); err != nil {
		p.Close()
		return err
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := p.Write([]byte("O\r")); err != nil {
		p.Close()
		return err
	}

	go sl.readLoop(ctx)
	return nil
}

func (sl *SLCan) Close() error {
	if sl.port == nil {
		return nil
	}
	time.Sleep(10 * time.Millisecond)
	sl.port.Write([]byte("C\r")) // best-effort channel close
	time.Sleep(10 * time.Millisecond)
	return sl.port.Close()
}

func (sl *SLCan) Send(ctx context.Context, f gocan.Frame) error {
	buf := make([]byte, 0, 5+int(f.Length)*2+1)
	buf = append(buf, 't')
	id := f.ID & 0x7FF
	buf = append(buf, nybbleToHex(byte(id>>8)), nybbleToHex(byte(id>>4)&0xF), nybbleToHex(byte(id)&0xF))
	buf = append(buf, nybbleToHex(f.Length&0xF))
	for i := range int(f.Length) {
		buf = append(buf, nybbleToHex(f.Data[i]>>4), nybbleToHex(f.Data[i]&0xF))
	}
	buf = append(buf, '\r')
	if _, err := sl.port.Write(buf); err != nil {
		return fmt.Errorf("failed to write to com port: %w", err)
	}
	if sl.cfg.Debug {
		sl.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: ">> " + string(buf)})
	}
	return nil
}

func (sl *SLCan) readLoop(ctx context.Context) {
	readBuf := make([]byte, 8)
	for {
		n, err := sl.port.Read(readBuf)
		if err != nil {
			if ctx.Err() == nil {
				sl.bus.Fatal(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		if n == 0 {
			continue
		}
		sl.parse(readBuf[:n])
	}
}

func (sl *SLCan) parse(data []byte) {
	for _, b := range data {
		if b != '\r' {
			sl.line = append(sl.line, b)
			continue
		}
		if len(sl.line) == 0 {
			continue
		}
		switch sl.line[0] {
		case 't':
			if sl.cfg.Debug {
				sl.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "<< " + string(sl.line)})
			}
			f, err := decodeFrame(sl.line)
			if err != nil {
				sl.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("%v: %X", err, sl.line)})
			} else {
				sl.bus.Deliver(f)
			}
		case 'z': // transmit ack, not gated on
		default:
			sl.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "Unknown>> " + string(sl.line)})
		}
		sl.line = sl.line[:0]
	}
}

func decodeFrame(buff []byte) (gocan.Frame, error) {
	if len(buff) < 5 {
		return gocan.Frame{}, fmt.Errorf("short frame")
	}
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode identifier: %w", err)
	}
	dlc, err := strconv.ParseUint(string(buff[4:5]), 16, 8)
	if err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode data length: %w", err)
	}
	if dlc > 8 || len(buff) < int(5+dlc*2) {
		return gocan.Frame{}, fmt.Errorf("invalid data length: %d", dlc)
	}
	f := gocan.Frame{ID: uint32(id), Length: uint8(dlc)}
	if _, err := hex.Decode(f.Data[:dlc], buff[5:5+dlc*2]); err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode frame body: %w", err)
	}
	return f, nil
}

func nybbleToHex(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + (n - 10)
}

func bitRate(rate float64) (string, error) {
	switch rate {
	case 10:
		return "S0", nil
	case 20:
		return "S1", nil
	case 50:
		return "S2", nil
	case 100:
		return "S3", nil
	case 125:
		return "S4", nil
	case 250:
		return "S5", nil
	case 500:
		return "S6", nil
	case 750:
		return "S7", nil
	case 1000:
		return "S8", nil
	case 615.384:
		return "S9", nil
	default:
		return "", fmt.Errorf("unsupported CAN rate: %g kbit/s", rate)
	}
}
