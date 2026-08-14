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
	"slices"
	"strconv"
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

// Hunt order: the power-up default (PP 0C = 115.2 kbps, also where an
// ATZ'd close leaves the device) first, then the target rate (a device
// left switched by a crashed session), then the rest. cfg.PortBaudrate,
// when set, is tried before all of these.
var baudrates = [...]uint{115200, 2_000_000, 38400, 230400, 921600, 1_000_000, 57600}

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

	// Hunt for the adapter's current baud rate and move it to 2 Mbit. A
	// failed handshake leaves the device reverted to its old rate (STBRT),
	// and a device mid-reboot answers nothing, so keep sweeping the
	// candidate rates until the overall deadline.
	const target = 2_000_000
	rates := make([]uint, 0, len(baudrates)+1)
	if st.cfg.PortBaudrate > 0 {
		rates = append(rates, uint(st.cfg.PortBaudrate))
	}
	for _, r := range baudrates {
		if !slices.Contains(rates, r) {
			rates = append(rates, r)
		}
	}
	deadline := time.Now().Add(4 * time.Second)
	var err error
hunt:
	for {
		for _, from := range rates {
			if err = st.trySpeed(from, target); err == nil {
				break hunt
			}
		}
		if time.Now().After(deadline) {
			st.port.Close()
			return fmt.Errorf("failed to switch adapter baudrate: %w", err)
		}
	}

	initCmds := []string{
		"ATE0",         // echo off (insurance; the probe already sent it)
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
		"STCMM1",       // ACK received frames (normal node): an unACKed ECU retransmits back-to-back, flooding the reply window with duplicates (seen on STN1130 v5.10.1)
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
		lines, err := st.exec(cmd, 200*time.Millisecond)
		if err != nil {
			st.port.Close()
			return fmt.Errorf("scantool init %q: %w", cmd, err)
		}
		// A '?' means the firmware rejected the command (e.g. STUFC on
		// non-stand-alone devices); surface it but keep going — refusing
		// to open over a single tuning command would regress devices
		// that work today.
		if !hasOK(lines) {
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: fmt.Sprintf("init %q answered %q", cmd, lines)})
		}
	}
	return nil
}

func (st *Scantool) Close() error {
	if st.port == nil {
		return nil
	}
	time.Sleep(25 * time.Millisecond)
	// Full reset, not ATWS: reusing a warm-started device showed CAN-level
	// misbehavior on STN1130 v5.10.1 (unACKed ECU frames retransmitted in
	// bursts, stale replies leaking into the next STPX window). ATZ reboots
	// to power-up defaults at 115.2 kbps; the next Open hunts it there.
	st.port.Write([]byte("ATWS\r"))
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
	if err := st.sendCommand(ctx, cmd.String(), wait); err != nil {
		// The port is our only link to the device; a failed exchange with no
		// shutdown in progress means it is gone.
		if ctx.Err() == nil && st.bus.Err() == nil {
			st.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: err.Error(), Err: err})
		}
		return err
	}
	return nil
}

// SetFilter reprograms the ATCF/ATCM acceptance filter at runtime; the
// protocol must be closed (STPC) while setting it.
func (st *Scantool) SetFilter(filters []uint32) error {
	st.filter, st.mask = canFilter(filters)
	for _, cmd := range []string{"STPC", st.mask, st.filter, "STPO"} {
		if err := st.sendCommand(context.Background(), cmd, 0); err != nil {
			return err
		}
	}
	return nil
}

// handleLine processes one completed response line and resets the buffer.
func (st *Scantool) handleLine() {
	msg := string(st.line)
	st.line = st.line[:0]
	if msg == "" {
		return
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
			return
		}
		st.bus.Deliver(f)
	}
}

// sendCommand writes cmd, then parses and delivers response lines as they
// arrive, up to the '>' prompt — frames received during a long STPX window
// reach the bus in real time, and the caller may cancel ctx once it has
// what it needs. wait is the on-wire reply wait (STPX t:) the prompt may
// lag behind; zero for commands that answer immediately. On cancellation
// mid-window the STN is interrupted so the next command starts clean.
func (st *Scantool) sendCommand(ctx context.Context, cmd string, wait time.Duration) error {
	st.port.ResetInputBuffer() // discard stale bytes from an interrupted window
	if _, err := st.port.Write([]byte(cmd + "\r")); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}
	deadline := time.Now().Add(wait + time.Second)
	st.line = st.line[:0]
	var readBuf [64]byte
	for {
		if err := context.Cause(ctx); err != nil && ctx.Err() != nil {
			st.interrupt()
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("timeout waiting for '>' prompt")
		}
		n, err := st.port.Read(readBuf[:])
		if err != nil {
			return fmt.Errorf("read from port: %w", err)
		}
		for _, b := range readBuf[:n] {
			switch b {
			case '>':
				st.handleLine()
				return nil
			case '\r':
				st.handleLine()
			default:
				st.line = append(st.line, b)
			}
		}
	}
}

// interrupt halts an in-progress STPX reception: any character stops it
// (the STN discards the character and answers STOPPED). Drain to the
// prompt so the next command starts clean. In the rare race where the
// window already closed, the bare CR repeats the last command; the next
// sendCommand's ResetInputBuffer clears whatever that produces.
func (st *Scantool) interrupt() {
	if _, err := st.port.Write([]byte{'\r'}); err != nil {
		return
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	var readBuf [64]byte
	for time.Now().Before(deadline) {
		n, err := st.port.Read(readBuf[:])
		if err != nil {
			return
		}
		if bytes.IndexByte(readBuf[:n], '>') >= 0 {
			return
		}
	}
}

// trySpeed probes the adapter at baud `from` and, if alive, walks the
// documented STBR handshake to move it to `to`: widen the handshake window
// with STBRT, send STBR and read its reply at the old baud ('?' = rate not
// within 3%), switch the host UART, read the STI banner the device prints
// at the new baud, and answer with a CR it must see before the STBRT window
// closes — otherwise it reverts to `from`, so any failure past STBR leaves
// the device at a known rate and the hunt can simply retry.
func (st *Scantool) trySpeed(from, to uint) error {
	if err := st.port.SetBaud(int(from)); err != nil {
		return err
	}
	if !st.probe() {
		return fmt.Errorf("no adapter at %d bps", from)
	}
	if from == to {
		// Already at the target; confirm it is an STN and grab the banner.
		lines, err := st.exec("STI", 200*time.Millisecond)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(lines, func(s string) bool { return bytes.Contains([]byte(s), []byte("STN")) })
		if i < 0 {
			return fmt.Errorf("device at %d bps is not an STN: %q", from, lines)
		}
		st.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: lines[i]})
		return nil
	}

	// 250 ms handshake window: enough for the host to switch and answer,
	// without the 1 s/connect penalty on firmware that delays the STI
	// banner by the full STBRT value.
	if lines, err := st.exec("STBRT250", 200*time.Millisecond); err != nil || !hasOK(lines) {
		return fmt.Errorf("STBRT at %d bps: %q %v", from, lines, err)
	}

	// STBR answers at the old baud before switching: OK, or '?' when the
	// rate cannot be generated. No prompt follows the OK — the next output
	// is the STI banner at the new baud. Only an explicit '?' aborts; a
	// missed OK (marginal UARTs mangle lines) falls through to the banner
	// hunt, which is the check that matters.
	st.port.ResetInputBuffer()
	if _, err := st.port.Write([]byte("STBR" + strconv.Itoa(int(to)) + "\r")); err != nil {
		return err
	}
	reply, err := st.readLine(200*time.Millisecond, func(line string) bool { return line == "OK" || line == "?" })
	if err == nil && reply == "?" {
		return fmt.Errorf("adapter cannot generate %d bps", to)
	}

	if err := st.port.SetBaud(int(to)); err != nil {
		return err
	}
	// The banner is printed ~75 ms after the switch; the generous deadline
	// covers firmware that scales the delay with STBRT.
	banner, err := st.readLine(1500*time.Millisecond, func(line string) bool { return bytes.Contains([]byte(line), []byte("STN")) })
	if err != nil {
		return fmt.Errorf("no STI banner at %d bps: %w", to, err)
	}
	// Confirm with a CR (it must land within the STBRT window or the device
	// reverts to the old rate), then prove the new rate actually works with
	// a probe rather than trusting one more fragile status line. If the
	// device did revert, the probe fails and the hunt simply retries.
	if _, err := st.port.Write([]byte{'\r'}); err != nil {
		return err
	}
	if !st.probe() {
		return fmt.Errorf("baudrate switch to %d bps not confirmed", to)
	}
	st.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: banner})
	return nil
}

// probe checks for a live device at the current host baud by turning echo
// off. ATE0 rather than a bare CR: an empty line repeats the last stored
// command, which could retransmit a stale STPX frame onto the CAN bus. Two
// tries — the first may land on a dirty device line buffer and answer '?'.
func (st *Scantool) probe() bool {
	for range 2 {
		if lines, err := st.exec("ATE0", 100*time.Millisecond); err == nil && hasOK(lines) {
			return true
		}
	}
	return false
}

// exec writes a control command and collects its response lines up to the
// '>' prompt. Unlike sendCommand it verifies instead of delivering: used
// for the probe, the baud handshake and the init sequence.
func (st *Scantool) exec(cmd string, timeout time.Duration) ([]string, error) {
	st.port.ResetInputBuffer()
	if _, err := st.port.Write([]byte(cmd + "\r")); err != nil {
		return nil, err
	}
	return st.readToPrompt(timeout)
}

// readToPrompt reads response lines until the '>' prompt or the deadline.
func (st *Scantool) readToPrompt(timeout time.Duration) ([]string, error) {
	var lines []string
	_, err := st.scanLines(timeout, func(line string, prompt bool) bool {
		if line != "" {
			lines = append(lines, line)
		}
		return prompt
	})
	return lines, err
}

// readLine reads until a line matches, without requiring a prompt.
func (st *Scantool) readLine(timeout time.Duration, match func(string) bool) (string, error) {
	return st.scanLines(timeout, func(line string, _ bool) bool { return match(line) })
}

// scanLines feeds completed lines (and prompt sightings, with an empty
// line) to done until it returns true or the deadline passes. The port
// read timeout is short, so the loop polls at that granularity.
func (st *Scantool) scanLines(timeout time.Duration, done func(line string, prompt bool) bool) (string, error) {
	deadline := time.Now().Add(timeout)
	var line []byte
	var readBuf [64]byte
	for time.Now().Before(deadline) {
		n, err := st.port.Read(readBuf[:])
		if err != nil {
			return "", err
		}
		for _, b := range readBuf[:n] {
			switch b {
			case '>', '\r', '\n':
				s := string(line)
				line = line[:0]
				if done(s, b == '>') {
					return s, nil
				}
			default:
				line = append(line, b)
			}
		}
	}
	return "", errors.New("timeout")
}

// hasOK reports whether one of the response lines is exactly "OK".
func hasOK(lines []string) bool {
	return slices.Contains(lines, "OK")
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
