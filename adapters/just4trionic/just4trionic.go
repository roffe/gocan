// Package just4trionic drives the Just4Trionic STM32F103C8T6 CAN adapter
// over its virtual COM port (115200 baud). Importing the package registers
// the "Just4Trionic" adapter.
//
// Wire protocol (Lawicel-flavoured ASCII, CR terminated):
//
//	ESC         leave/reset mode (sent on open and close)
//	O / C       open / close the CAN channel
//	Sn / s2     set bit-rate (s2 = 615.384 kbit/s for Trionic 5)
//	Mxxxxxxxx   acceptance code, mxxxxxxxx acceptance mask
//	t<id><l><data>  transmit: unpadded hex id, ascii DLC, data padded to 8 bytes
//
// Received frames arrive as LF-terminated lines "wiiil dd..": 'w', 3 hex id
// chars, ascii DLC and the data in hex.
package just4trionic

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
		Name:               "Just4Trionic",
		Description:        "STM32F103C8T6 based CAN adapter",
		RequiresSerialPort: true,
		Capabilities:       gocan.Capabilities{HSCAN: true},
		New:                New,
	})
}

type Just4Trionic struct {
	cfg     gocan.Config
	bus     *gocan.Bus
	port    serial.Port
	canRate string
	line    []byte
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	a := &Just4Trionic{cfg: cfg}
	rate, err := canRate(cfg.CANRate)
	if err != nil {
		return nil, err
	}
	a.canRate = rate
	return a, nil
}

func (a *Just4Trionic) Open(ctx context.Context, bus *gocan.Bus) error {
	a.bus = bus
	p, err := serial.Open(a.cfg.Port, &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("failed to open com port %q: %w", a.cfg.Port, err)
	}
	p.SetReadTimeout(1 * time.Millisecond)
	a.port = p
	p.ResetOutputBuffer()

	code, mask := acceptanceFilters(a.cfg.CANFilter)
	cmds := []string{
		"\x1B", // empty buffer / leave any mode
		"O",    // enter canbus mode
		code,
		mask,
		a.canRate,
	}
	for n, c := range cmds {
		if n == 3 {
			p.ResetInputBuffer()
		}
		if a.cfg.Debug {
			bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: ">> " + c})
		}
		if _, err := p.Write([]byte(c + "\r")); err != nil {
			p.Close()
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}

	go a.readLoop(ctx)
	return nil
}

func (a *Just4Trionic) Close() error {
	if a.port == nil {
		return nil
	}
	time.Sleep(50 * time.Millisecond)
	a.port.Write([]byte("\x1B")) // best-effort mode reset
	time.Sleep(10 * time.Millisecond)
	return a.port.Close()
}

// Send encodes and writes one frame; the write completing is the confirmation
// (the firmware has no TX ack).
func (a *Just4Trionic) Send(ctx context.Context, f gocan.Frame) error {
	out := "t" + strconv.FormatUint(uint64(f.ID), 16) +
		strconv.Itoa(int(f.Length)) +
		hex.EncodeToString(f.Data[:f.Length])
	for i := int(f.Length); i < 8; i++ {
		out += "00"
	}
	out += "\r"
	if _, err := a.port.Write([]byte(out)); err != nil {
		return fmt.Errorf("failed to write to com port: %q, %w", out, err)
	}
	if a.cfg.Debug {
		a.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: ">> " + out})
	}
	return nil
}

// SetFilter reprograms the acceptance filter; the channel must be closed
// while setting M/m, so bounce C -> M -> m -> O.
func (a *Just4Trionic) SetFilter(filters []uint32) error {
	code, mask := acceptanceFilters(filters)
	for _, c := range []string{"C", code, mask, "O"} {
		if _, err := a.port.Write([]byte(c + "\r")); err != nil {
			return err
		}
	}
	return nil
}

func (a *Just4Trionic) readLoop(ctx context.Context) {
	readBuffer := make([]byte, 8)
	for {
		n, err := a.port.Read(readBuffer)
		if err != nil {
			if ctx.Err() == nil {
				a.bus.Fatal(fmt.Errorf("failed to read com port: %w", err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		if n == 0 {
			continue
		}
		a.parse(readBuffer[:n])
	}
}

func (a *Just4Trionic) parse(data []byte) {
	for _, b := range data {
		if (b == 0x0D || b == 0x0A) && len(a.line) == 0 {
			continue
		}
		if b == 0x0A {
			if a.line[0] == 'w' {
				f, err := decodeFrame(a.line[1 : len(a.line)-1])
				if err != nil {
					a.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("failed to decode frame: %v %X", err, a.line)})
				} else {
					a.bus.Deliver(f)
				}
			} else if a.cfg.Debug {
				a.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "<< " + string(a.line)})
			}
			a.line = a.line[:0]
			continue
		}
		a.line = append(a.line, b)
	}
}

// decodeFrame parses "iiil dd.." (3 hex id chars, ascii DLC, hex data).
func decodeFrame(buff []byte) (gocan.Frame, error) {
	if len(buff) < 4 {
		return gocan.Frame{}, fmt.Errorf("short frame %q", buff)
	}
	id, err := strconv.ParseUint(string(buff[0:3]), 16, 32)
	if err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode identifier: %w", err)
	}
	dlc := int(buff[3] - 0x30)
	if dlc < 0 || dlc > 8 || len(buff) < 4+dlc*2 {
		return gocan.Frame{}, fmt.Errorf("bad DLC in %q", buff)
	}
	f := gocan.Frame{ID: uint32(id), Length: uint8(dlc)}
	if _, err := hex.Decode(f.Data[:dlc], buff[4:4+dlc*2]); err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode frame body: %w", err)
	}
	return f, nil
}

// canRate maps a CAN rate in kbit/s to the firmware's Sn command; s2 is the
// board's custom 615.384 kbit/s (Trionic 5) rate.
func canRate(rate float64) (string, error) {
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
	case 615.384:
		return "s2", nil
	case 800:
		return "S7", nil
	case 1000:
		return "S8", nil
	default:
		return "", fmt.Errorf("unknown rate: %f", rate)
	}
}

// acceptanceFilters computes the M/m commands covering all given 11-bit IDs.
func acceptanceFilters(idList []uint32) (string, string) {
	if len(idList) == 1 && idList[0] == 0 {
		return "\r", "\r"
	}
	var code, mask uint32
	if len(idList) == 0 {
		mask = ^uint32(0)
	} else {
		// matches the shipped v1 behavior: code stays 0 (it zeroed the
		// accumulator before AND-ing), only the mask varies with the ids
		for _, canID := range idList {
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16
	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}
