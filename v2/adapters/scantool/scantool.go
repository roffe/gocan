// Package scantool drives ScanTool.net STN11xx/STN21xx based adapters
// (OBDLink SX/EX, generic STN1170/STN2120 boards) over their virtual COM
// port. Importing the package registers the "OBDLink SX", "OBDLink EX",
// "STN1170" and "STN2120" adapters.
//
// The device is a command/response ELM-compatible interpreter: frames are
// transmitted with STPX (hex header/data, optional t: timeout and r: reply
// count) and any reply frames come back as response lines terminated by the
// '>' prompt — there is no free-running monitor. Send therefore performs the
// whole exchange synchronously and delivers reply frames to the bus before
// returning; the reply-count hint is taken from gocan.ExpectedResponses and
// the timeout from the context deadline.
//
// Open hunts for the adapter across the known baud rates and switches it to
// 2 Mbit with STBR before running the init sequence (echo/spaces off, STP
// protocol + bit-rate, headers on, flow control off, ATCF/ATCM filter).
package scantool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

const (
	OBDLinkSX = "OBDLink SX"
	OBDLinkEX = "OBDLink EX"
	STN1170   = "STN1170"
	STN2120   = "STN2120"
)

var baudrates = [...]uint{115200, 38400, 230400, 921600, 2000000, 1000000, 57600}

// defaultReplyWait mirrors the STPTO250 device default set during init.
const defaultReplyWait = 250 * time.Millisecond

func init() {
	register := func(name, desc string, caps gocan.Capabilities) {
		gocan.Register(gocan.AdapterInfo{
			Name:               name,
			Description:        desc,
			RequiresSerialPort: true,
			Capabilities:       caps,
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				return New(name, cfg)
			},
		})
	}
	register(OBDLinkSX, "ScanTool.net "+OBDLinkSX, gocan.Capabilities{HSCAN: true})
	register(OBDLinkEX, "ScanTool.net "+OBDLinkEX, gocan.Capabilities{HSCAN: true})
	register(STN1170, "ScanTool.net STN1170 based adapter", gocan.Capabilities{HSCAN: true, SWCAN: true, KLine: true})
	register(STN2120, "ScanTool.net STN2120 based adapter", gocan.Capabilities{HSCAN: true, SWCAN: true, KLine: true})
}

// port is the transport under the STN command interpreter; implemented by
// the VCP serial port and (with the ftdi tag) the D2XX wrapper.
type port interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	SetBaud(baud int) error
	SetReadTimeout(t time.Duration) error
	ResetInputBuffer() error
	ResetOutputBuffer() error
}

// vcpPort adapts go.bug.st/serial to the port interface.
type vcpPort struct {
	serial.Port
}

func (v vcpPort) SetBaud(baud int) error {
	return v.SetMode(&serial.Mode{
		BaudRate: baud,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	})
}

type Scantool struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	name string

	protocolCMD, canrateCMD string
	filter, mask            string

	port     port
	openPort func() (port, error) // opens the transport; nil = VCP from cfg.Port
	line     []byte               // response accumulator reused across reads
}

func New(name string, cfg gocan.Config) (gocan.Adapter, error) {
	st := &Scantool{cfg: cfg, name: name}
	var err error
	st.protocolCMD, st.canrateCMD, err = canRateCommands(name, cfg.CANRate)
	if err != nil {
		return nil, err
	}
	st.filter, st.mask = canFilter(cfg.CANFilter)
	return st, nil
}

func (st *Scantool) Open(ctx context.Context, bus *gocan.Bus) error {
	st.bus = bus
	if st.port == nil {
		if st.openPort == nil { // default: VCP from cfg.Port
			st.openPort = func() (port, error) {
				sp, err := serial.Open(st.cfg.Port, &serial.Mode{
					BaudRate: st.cfg.PortBaudrate,
					Parity:   serial.NoParity,
					DataBits: 8,
					StopBits: serial.OneStopBit,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to open com port %q: %w", st.cfg.Port, err)
				}
				return vcpPort{sp}, nil
			}
		}
		p, err := st.openPort()
		if err != nil {
			return err
		}
		st.port = p
	}
	if err := st.port.SetReadTimeout(10 * time.Millisecond); err != nil {
		st.port.Close()
		return err
	}

	// Hunt for the adapter's current baud rate and move it to 2 Mbit.
	const target = 2_000_000
	found := false
	for _, from := range baudrates {
		if err := st.trySpeed(from, target); err == nil {
			found = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !found {
		st.port.Close()
		return errors.New("failed to switch adapter baudrate")
	}

	time.Sleep(50 * time.Millisecond)

	initCmds := []string{
		"ATE0",         // echo off
		"STUFC0",       // flow control off
		"ATS0",         // spaces off
		"ATV1",         // variable DLC on
		st.protocolCMD, // CAN protocol
		"ATH1",         // headers on
		"ATAT0",        // adaptive timing off
		"ATCAF0",       // automatic formatting off
		st.canrateCMD,  // CAN bit-rate (may be empty)
		"ATCFC0",       // automatic CAN flow control off
		"STPTO250",     // default reply wait 250 ms (STPX t: only sent when it differs)
		"ATR0",         // replies off
		st.mask,
		st.filter,
	}
	for _, cmd := range initCmds {
		if cmd == "" {
			continue
		}
		if st.cfg.Debug {
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: ">> " + cmd})
		}
		if _, err := st.port.Write([]byte(cmd + "\r")); err != nil {
			st.port.Close()
			return fmt.Errorf("scantool init write failed: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := st.port.ResetInputBuffer(); err != nil {
		st.port.Close()
		return err
	}
	return nil
}

func (st *Scantool) Close() error {
	if st.port == nil {
		return nil
	}
	time.Sleep(25 * time.Millisecond)
	st.port.Write([]byte("ATZ\r")) // best-effort reset
	time.Sleep(10 * time.Millisecond)
	st.port.ResetInputBuffer()
	st.port.ResetOutputBuffer()
	return st.port.Close()
}

// Send transmits one frame with STPX and processes the response lines. Reply
// frames ride in the command response (there is no monitor mode), so they are
// delivered to the bus here before Send returns.
func (st *Scantool) Send(ctx context.Context, f gocan.Frame) error {
	var cmd bytes.Buffer
	fmt.Fprintf(&cmd, "STPXh:%03x,d:", f.ID&0xFFF)
	fmt.Fprintf(&cmd, "%x", f.Bytes())
	// The ctx deadline is cancellation only, never a wire timeout: the reply
	// wait is the WithResponseTimeout hint (Request stamps it from a near
	// deadline) or the STPTO250 device default. t: rides along only when it
	// differs (ceiled to whole ms, so deadline-derived ~249.9 ms is 250 and
	// stays silent). STPX t: is 16-bit ms, larger values make the STN reject
	// the whole command with '?'.
	wait := defaultReplyWait
	if n := gocan.ExpectedResponses(ctx); n > 0 {
		if d := gocan.ResponseTimeout(ctx); d > 0 {
			wait = min(d+time.Millisecond-1, 65535*time.Millisecond).Truncate(time.Millisecond)
			if wait != defaultReplyWait {
				fmt.Fprintf(&cmd, ",t:%d", wait.Milliseconds())
			}
		}
		fmt.Fprintf(&cmd, ",r:%d", n)
	}

	if st.cfg.Debug {
		st.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "<o> " + cmd.String()})
	}
	resp, err := st.sendCommand(ctx, cmd.String(), wait)
	if err != nil {
		// The port is our only link to the device; a failed exchange with no
		// shutdown in progress means it is gone.
		if ctx.Err() == nil && st.bus.Err() == nil {
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
		}
		return err
	}
	st.handleResponse(resp)
	return nil
}

// SetFilter reprograms the ATCF/ATCM acceptance filter at runtime; the
// protocol must be closed (STPC) while setting it.
func (st *Scantool) SetFilter(filters []uint32) error {
	st.filter, st.mask = canFilter(filters)
	for _, cmd := range []string{"STPC", st.mask, st.filter, "STPO"} {
		if _, err := st.sendCommand(context.Background(), cmd, 0); err != nil {
			return err
		}
	}
	return nil
}

func (st *Scantool) handleResponse(resp string) {
	for msg := range strings.SplitSeq(strings.TrimSuffix(resp, "\r\r"), "\r") {
		if msg == "" {
			continue
		}
		if st.cfg.Debug {
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "<i> " + msg})
		}
		switch msg {
		case "CAN ERROR":
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: "CAN ERROR"})
		case "STOPPED":
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "STOPPED"})
		case "?":
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: "UNKNOWN COMMAND"})
		case "NO DATA", "OK":
		default:
			f, err := decodeFrame(msg)
			if err != nil {
				st.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("failed to decode frame %q: %v", msg, err)})
				continue
			}
			st.bus.Deliver(f)
		}
	}
}

// sendCommand writes cmd and reads the full response up to the '>' prompt.
// wait is the on-wire reply wait (STPX t:) the prompt may lag behind; zero
// for commands that answer immediately.
func (st *Scantool) sendCommand(ctx context.Context, cmd string, wait time.Duration) (string, error) {
	if _, err := st.port.Write([]byte(cmd + "\r")); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}
	deadline := time.Now().Add(wait + time.Second)
	st.line = st.line[:0]
	var readBuf [64]byte
	for {
		if err := context.Cause(ctx); err != nil && ctx.Err() != nil {
			return "", err
		}
		if time.Now().After(deadline) {
			return "", errors.New("timeout waiting for '>' prompt")
		}
		n, err := st.port.Read(readBuf[:])
		if err != nil {
			return "", fmt.Errorf("read from port: %w", err)
		}
		for _, b := range readBuf[:n] {
			if b == '>' {
				return string(st.line), nil
			}
			st.line = append(st.line, b)
		}
	}
}

// trySpeed pokes the adapter at baud `from` and asks it to move to `to` with
// STBR, confirming by reading the STN identification string.
func (st *Scantool) trySpeed(from, to uint) error {
	if err := st.port.SetBaud(int(from)); err != nil {
		return err
	}
	if _, err := st.port.Write([]byte{'\r', '\r', '\r'}); err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)

	if _, err := st.port.Write([]byte("STBR" + strconv.Itoa(int(to)) + "\r")); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)

	if err := st.port.ResetInputBuffer(); err != nil {
		return err
	}
	if err := st.port.SetBaud(int(to)); err != nil {
		return err
	}

	readBuf := make([]byte, 64)
	line := make([]byte, 0, 128)
	for range 10 {
		n, err := st.port.Read(readBuf)
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(4 * time.Millisecond)
			continue
		}
		for _, b := range readBuf[:n] {
			if b == '\r' {
				if len(line) == 0 {
					continue
				}
				if bytes.Contains(line, []byte("STN")) {
					st.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: string(line)})
					_, err := st.port.Write([]byte{'\r'})
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

// decodeFrame parses a response line "iiiDDDD.." (3 hex id + hex data).
func decodeFrame(msg string) (gocan.Frame, error) {
	if len(msg) < 3 {
		return gocan.Frame{}, fmt.Errorf("short frame %q", msg)
	}
	id, err := strconv.ParseUint(msg[:3], 16, 32)
	if err != nil {
		return gocan.Frame{}, fmt.Errorf("failed to decode identifier: %w", err)
	}
	body := msg[3:]
	if len(body)%2 != 0 || len(body) > 16 {
		return gocan.Frame{}, fmt.Errorf("bad frame body %q", msg)
	}
	f := gocan.Frame{ID: uint32(id), Length: uint8(len(body) / 2)}
	for i := 0; i < len(body); i += 2 {
		v, err := strconv.ParseUint(body[i:i+2], 16, 8)
		if err != nil {
			return gocan.Frame{}, fmt.Errorf("bad frame body %q: %w", msg, err)
		}
		f.Data[i/2] = byte(v)
	}
	return f, nil
}

// canRateCommands maps the CAN rate to the STP protocol + bit-rate commands.
func canRateCommands(name string, rate float64) (protocolCMD, canrateCMD string, err error) {
	switch rate {
	case 33.3: // SWCAN, STN1170 & STN2120 only
		return "STP61", "STCSWM2", nil
	case 500:
		return "STP33", "", nil
	case 615.384:
		switch name {
		case OBDLinkSX, STN1170:
			return "STP33", "STCTR8101FC", nil
		case OBDLinkEX, STN2120:
			return "STP33", "STCTR82239F", nil
		default:
			return "", "", fmt.Errorf("unhandled adapter: %s", name)
		}
	default:
		return "", "", fmt.Errorf("unhandled CANBus rate: %f", rate)
	}
}

// canFilter computes the ATCF filter / ATCM mask pair covering all ids (a
// superset may pass; the bus's subscription dispatch drops the rest).
func canFilter(ids []uint32) (filter, mask string) {
	var filt uint32 = 0xFFF
	var m uint32 = 0x000
	for _, id := range ids {
		filt &= id
		m |= id
	}
	m = (^m & 0x7FF) | filt
	if len(ids) == 0 {
		filt = 0
		m = 0x7FF
	}
	return fmt.Sprintf("ATCF%03X", filt), fmt.Sprintf("ATCM%03X", m)
}
