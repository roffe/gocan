// Package elm327 drives generic ELM327 CAN adapters over their virtual COM
// port. Importing the package registers the "ELM327" adapter. EXPERIMENTAL,
// like the v1 driver it replaces.
//
// The ELM327 is a command/response AT interpreter: the transmit header is
// set with ATSH when the target ID changes, ATR0/ATR1 toggles whether the
// device listens for replies (driven by the gocan.ExpectedResponses hint),
// and the frame payload is sent as bare hex. Reply frames come back as
// response lines before the '>' prompt, so Send performs the whole exchange
// and delivers replies to the bus before returning.
package elm327

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

const (
	respOK      = "OK\r\r"
	promptByte  = '>'
	cmdDeadline = time.Second
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:               "ELM327",
		Description:        "ELM327 CANBus Adapter",
		RequiresSerialPort: true,
		Capabilities:       gocan.Capabilities{HSCAN: true, KLine: true},
		New:                New,
	})
}

type ELM327 struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	port serial.Port

	currentID uint32
	response  bool
	line      []byte
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &ELM327{cfg: cfg}, nil
}

func (el *ELM327) Open(ctx context.Context, bus *gocan.Bus) error {
	el.bus = bus
	p, err := serial.Open(el.cfg.Port, &serial.Mode{
		BaudRate: el.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("failed to open com port %q: %w", el.cfg.Port, err)
	}
	el.port = p
	p.SetReadTimeout(10 * time.Millisecond)
	p.ResetOutputBuffer()
	p.ResetInputBuffer()

	if err := el.init(); err != nil {
		el.port.Close()
		return fmt.Errorf("failed to init ELM327: %w", err)
	}
	// No read loop: the ELM only talks in response to commands, so all
	// receiving happens inside Send.
	return nil
}

func (el *ELM327) Close() error {
	if el.port == nil {
		return nil
	}
	el.port.Write([]byte("ATZ\r")) // best-effort reset
	time.Sleep(100 * time.Millisecond)
	el.port.Write([]byte("ATZ\r"))
	time.Sleep(100 * time.Millisecond)
	return el.port.Close()
}

func (el *ELM327) init() error {
	el.writePort("ATZ")
	time.Sleep(1 * time.Second)
	el.port.ResetInputBuffer()

	commands := []string{
		"ATE0",    // Echo off
		"ATS0",    // Spaces off
		"ATL0",    // Linefeeds off
		"ATAL",    // Allow long messages
		"ATSP6",   // Set protocol to CAN 11 bit ID 500 kbps
		"ATH1",    // Headers on
		"ATAT2",   // Adaptive Timing
		"ATV1",    // Variable DLC on
		"ATR0",    // Responses off
		"ATAR",    // Automatic receive
		"ATCAF0",  // Automatic formatting off
		"ATCFC0",  // CAN flow control off
		"ATBRT28", // Set baud rate switch timeout to 40 ms
		"ATST32",  // Set read timeout to 200ms (hh*4ms)
	}
	for _, cmd := range commands {
		resp, err := el.sendCommand(cmd)
		if err != nil {
			return fmt.Errorf("error sending command %q: %w", cmd, err)
		}
		if !strings.HasSuffix(resp, respOK) {
			return fmt.Errorf("error sending command %q: %q", cmd, resp)
		}
	}

	if err := el.setFilter(el.cfg.CANFilter); err != nil {
		return fmt.Errorf("failed to set filter: %w", err)
	}

	if el.cfg.PortBaudrate != 500000 {
		if err := el.changeDeviceBaudrate(el.cfg.PortBaudrate, 500000); err != nil {
			return fmt.Errorf("failed to change speed: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	el.port.ResetInputBuffer()
	el.port.ResetOutputBuffer()
	return nil
}

// Send transmits one frame and processes the response lines; reply frames
// are delivered to the bus before returning.
func (el *ELM327) Send(ctx context.Context, f gocan.Frame) error {
	if el.currentID != f.ID {
		if err := el.setHeader(f.ID); err != nil {
			return err
		}
	}
	wantResponse := gocan.ExpectedResponses(ctx) > 0
	if wantResponse != el.response {
		if err := el.setResponse(wantResponse); err != nil {
			return err
		}
	}

	resp, err := el.sendCommand(fmt.Sprintf("%02X", f.Bytes()))
	if err != nil {
		return fmt.Errorf("failed to send frame: %w", err)
	}
	if resp == "\r" {
		return nil
	}
	for msg := range strings.SplitSeq(strings.TrimSuffix(resp, "\r\r"), "\r") {
		switch msg {
		case "", "OK":
			continue
		case "NO DATA":
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: "CAN ERROR"})
			continue
		case "STOPPED":
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "STOPPED"})
			continue
		case "?":
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "UNKNOWN COMMAND"})
			continue
		}
		if len(msg) < 4 {
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: "message invalid: " + msg})
			continue
		}
		id, err := strconv.ParseUint(msg[0:3], 16, 32)
		if err != nil {
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("invalid id in message %q: %v", msg, err)})
			continue
		}
		body := msg[3:]
		if len(body)%2 != 0 || len(body) > 16 {
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("invalid data in message %q", msg)})
			continue
		}
		rf := gocan.Frame{ID: uint32(id), Length: uint8(len(body) / 2)}
		if _, err := hex.Decode(rf.Data[:rf.Length], []byte(body)); err != nil {
			el.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("invalid data in message %q: %v", msg, err)})
			continue
		}
		el.bus.Deliver(rf)
	}
	return nil
}

func (el *ELM327) setHeader(id uint32) error {
	resp, err := el.sendCommand(fmt.Sprintf("ATSH%03X", id))
	if err != nil {
		return fmt.Errorf("failed to set header for ID %03X: %w", id, err)
	}
	if resp != respOK {
		return fmt.Errorf("failed to set header for ID %03X: %q", id, resp)
	}
	el.currentID = id
	return nil
}

func (el *ELM327) setResponse(enabled bool) error {
	cmd := "ATR0"
	if enabled {
		cmd = "ATR1"
	}
	resp, err := el.sendCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to set response required: %w", err)
	}
	if resp != respOK {
		return fmt.Errorf("failed to set response required: %q", resp)
	}
	el.response = enabled
	return nil
}

func (el *ELM327) setFilter(ids []uint32) error {
	filt := uint32(0xFFF)
	mask := uint32(0x000)
	if len(ids) > 0 {
		for _, id := range ids {
			filt &= id
			mask |= id
		}
		mask = (^mask & 0x7FF) | filt
	}
	for _, cmd := range []string{fmt.Sprintf("ATCF%03X", filt), fmt.Sprintf("ATCM%03X", mask)} {
		resp, err := el.sendCommand(cmd)
		if err != nil {
			return fmt.Errorf("error setting filter: %w", err)
		}
		if resp != respOK {
			return fmt.Errorf("error setting filter: %q", resp)
		}
	}
	return nil
}

func (el *ELM327) sendCommand(cmd string) (string, error) {
	if err := el.writePort(cmd); err != nil {
		return "", err
	}
	deadline := time.Now().Add(cmdDeadline)
	el.line = el.line[:0]
	var readBuf [64]byte
	for {
		if time.Now().After(deadline) {
			return "", errors.New("timeout waiting for '>' prompt")
		}
		n, err := el.port.Read(readBuf[:])
		if err != nil {
			return "", fmt.Errorf("read from port: %w", err)
		}
		for _, b := range readBuf[:n] {
			if b == promptByte {
				return string(el.line), nil
			}
			el.line = append(el.line, b)
		}
	}
}

func (el *ELM327) writePort(cmd string) error {
	n, err := el.port.Write([]byte(cmd + "\r"))
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}
	if n != len(cmd)+1 {
		return fmt.Errorf("failed to send full command, sent %d of %d bytes", n, len(cmd)+1)
	}
	return nil
}

func (el *ELM327) changeDeviceBaudrate(from, to int) error {
	mode := &serial.Mode{
		BaudRate: from,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}
	if err := el.port.SetMode(mode); err != nil {
		return err
	}
	divider := int(math.Round(4000000.0 / float64(to)))
	el.writePort(fmt.Sprintf("ATBRD%02X", divider))
	time.Sleep(50 * time.Millisecond)

	if err := el.port.ResetInputBuffer(); err != nil {
		return err
	}
	mode.BaudRate = to
	if err := el.port.SetMode(mode); err != nil {
		return err
	}

	readBuf := make([]byte, 64)
	line := make([]byte, 0, 128)
	for range 10 {
		n, err := el.port.Read(readBuf)
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		for _, b := range readBuf[:n] {
			if b == '\r' {
				if len(line) == 0 {
					continue
				}
				if bytes.Contains(line, []byte("ELM327")) {
					el.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: string(line)})
					_, err := el.port.Write([]byte{'\r'})
					return err
				}
				line = line[:0]
				continue
			}
			line = append(line, b)
		}
	}
	return fmt.Errorf("failed to change adapter baudrate from %d to %d bps", from, to)
}
