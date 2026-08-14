// Package yaca drives the YACA ("Yet Another CANbus Adapter") over its
// virtual COM port. Importing the package registers the "YACA" adapter.
//
// Wire protocol (Lawicel-flavoured ASCII): Sn selects one of the four fixed
// bit-rates (33.3 / 47.619 / 500 / 615.384 kbit/s), M/m program the SJA1000
// style acceptance code/mask, O/C open/close the channel. Transmit is
// "t iii l dd..\r"; received lines are LF terminated: "t..." frames, "F.."
// status flags and BEL for an unknown command.
package yaca

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:               "YACA",
		Description:        "Yet Another CANBus Adapter",
		RequiresSerialPort: true,
		Capabilities:       gocan.Capabilities{HSCAN: true},
		New:                New,
	})
}

type YACA struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	port serial.Port
	line []byte
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &YACA{cfg: cfg}, nil
}

func (ya *YACA) Open(ctx context.Context, bus *gocan.Bus) error {
	ya.bus = bus
	p, err := serial.Open(ya.cfg.Port, &serial.Mode{
		BaudRate: ya.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("failed to open com port %q: %w", ya.cfg.Port, err)
	}
	p.SetReadTimeout(1 * time.Millisecond)
	ya.port = p
	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	switch ya.cfg.CANRate {
	case 33.3:
		p.Write([]byte("S0\r"))
	case 47.619:
		p.Write([]byte("S1\r"))
	case 500:
		p.Write([]byte("S2\r"))
	case 615.384:
		p.Write([]byte("S3\r"))
	}
	time.Sleep(5 * time.Millisecond)

	code, mask := filterCodeAndMask(ya.cfg.CANFilter)
	p.Write([]byte(code + "\r"))
	time.Sleep(5 * time.Millisecond)
	p.Write([]byte(mask + "\r"))
	time.Sleep(5 * time.Millisecond)
	p.Write([]byte("O\r"))

	go ya.readLoop(ctx)
	return nil
}

func (ya *YACA) Close() error {
	if ya.port == nil {
		return nil
	}
	time.Sleep(10 * time.Millisecond)
	ya.port.Write([]byte("C\r")) // best-effort channel close
	time.Sleep(10 * time.Millisecond)
	return ya.port.Close()
}

func (ya *YACA) Send(ctx context.Context, f gocan.Frame) error {
	out := fmt.Sprintf("t%03x%d%s\x0D", f.ID&0xFFF, f.Length, hex.EncodeToString(f.Bytes()))
	if _, err := ya.port.Write([]byte(out)); err != nil {
		return fmt.Errorf("failed to write to com port: %s, %w", out, err)
	}
	if ya.cfg.Debug {
		ya.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: ">> " + out})
	}
	return nil
}

// SetFilter reprograms the acceptance filter; the channel must be closed
// while setting M/m, so bounce C -> M -> m -> O.
func (ya *YACA) SetFilter(filters []uint32) error {
	code, mask := filterCodeAndMask(filters)
	for _, c := range []string{"C", code, mask, "O"} {
		if _, err := ya.port.Write([]byte(c + "\r")); err != nil {
			return err
		}
	}
	return nil
}

func (ya *YACA) readLoop(ctx context.Context) {
	readBuffer := make([]byte, 8)
	for {
		n, err := ya.port.Read(readBuffer)
		if err != nil {
			if ctx.Err() == nil {
				ya.bus.Fatal(fmt.Errorf("failed to read from com port: %w", err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		if n == 0 {
			continue
		}
		ya.parse(readBuffer[:n])
	}
}

func (ya *YACA) parse(data []byte) {
	for _, b := range data {
		if b != '\n' {
			ya.line = append(ya.line, b)
			continue
		}
		if len(ya.line) == 0 {
			continue
		}
		switch ya.line[0] {
		case 'F':
			if err := decodeStatus(ya.line); err != nil {
				ya.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: fmt.Sprintf("CAN status error: %v", err)})
			}
		case 't':
			f, err := decodeFrame(ya.line)
			if err != nil {
				ya.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("failed to decode frame: %X", ya.line)})
			} else {
				ya.bus.Deliver(f)
			}
		case 0x07: // bell, last command was unknown
			ya.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "unknown command"})
		default:
			ya.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "Unknown>> " + string(ya.line)})
		}
		ya.line = ya.line[:0]
	}
}

// decodeFrame parses "tiiil dd.." — id in [1:4], DLC digit at [4] skipped in
// favour of the body length, data from [5].
func decodeFrame(buff []byte) (gocan.Frame, error) {
	if len(buff) < 5 {
		return gocan.Frame{}, fmt.Errorf("short frame")
	}
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode identifier: %w", err)
	}
	body := buff[5:]
	if len(body)%2 != 0 || len(body) > 16 {
		return gocan.Frame{}, fmt.Errorf("bad frame body")
	}
	f := gocan.Frame{ID: uint32(id), Length: uint8(len(body) / 2)}
	if _, err := hex.Decode(f.Data[:len(body)/2], body); err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode frame body: %w", err)
	}
	return f, nil
}

func decodeStatus(b []byte) error {
	v, err := strconv.ParseUint(string(b[1:]), 16, 16)
	if err != nil {
		return fmt.Errorf("failed to decode status: %w", err)
	}
	flags := []struct {
		bit int
		msg string
	}{
		{1, "CAN receive FIFO queue full"},
		{2, "CAN transmit FIFO queue full"},
		{3, "error warning (EI)"},
		{4, "data overrun (DOI)"},
		{5, "not used"},
		{6, "error passive (EPI)"},
		{7, "arbitration lost (ALI)"},
		{8, "bus error (BEI)"},
	}
	for _, f := range flags {
		if v&(1<<(f.bit-1)) != 0 {
			return errors.New(f.msg)
		}
	}
	return nil
}

// filterCodeAndMask computes the M/m commands: code from the lowest id,
// mask marking the bits that differ across ids (both shifted to the SJA1000
// 29-bit-register layout for 11-bit ids).
func filterCodeAndMask(data []uint32) (string, string) {
	var min uint32 = 0xffffffff
	for _, val := range data {
		if val < min {
			min = val
		}
	}
	if len(data) == 0 {
		min = 0
	}
	bitcount := make([]uint8, 32)
	for _, id := range data {
		for p, bit := range fmt.Sprintf("%032b", id) {
			if bit == '1' {
				bitcount[p]++
			}
		}
	}
	noIds := uint8(len(data))
	var mask uint32
	for i, bit := range bitcount {
		if bit == 0 {
			continue
		}
		if bit > 0 && bit < noIds {
			mask |= 1 << (31 - i)
		}
	}
	code := min<<21 | 0x0000FFFF
	mask = mask<<21 | 0x0000FFFF
	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}
