// Package txbridge drives the txbridge dongle over TCP (192.168.4.1:1337 or
// tcp://host[:1337] from cfg.Port). Importing the package registers the
// "txbridge wifi" adapter.
//
// The wire protocol is framed serial commands (command byte, length, data,
// 1-byte sum checksum — see gocan v1's pkg/serialcommand): 't' carries CAN
// frames (idHi, idLo, dlc, data), 'e' errors, and the dongle's host-side
// features (fast logger streams, WBL readings, RAM read/write) ride on their
// own command bytes ('r', 'R', 'w', 'W', ...). Those are exposed with the
// Command/Raw/Subscribe/Request methods on the concrete type; consumers
// reach them via bus.Adapter() and a type assertion.
package txbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"golang.org/x/mod/semver"
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:               "txbridge wifi",
		Description:        "txbridge over wifi",
		RequiresSerialPort: true,
		SerialPortOptional: true,
		Capabilities:       gocan.Capabilities{HSCAN: true},
		New:                New,
	})
}

type Txbridge struct {
	cfg  gocan.Config
	bus  *gocan.Bus
	port io.ReadWriteCloser

	writeMu sync.Mutex // one writer at a time so command framing never tears

	subMu sync.Mutex
	subs  map[*commandSub]struct{}
}

// commandSub is one Subscribe listener: a set of command bytes and its channel.
type commandSub struct {
	ctx  context.Context
	cmds map[byte]struct{}
	ch   chan *serialcommand.SerialCommand
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &Txbridge{cfg: cfg, subs: make(map[*commandSub]struct{})}, nil
}

func (tx *Txbridge) Open(ctx context.Context, bus *gocan.Bus) error {
	tx.bus = bus
	if tx.port != nil { // pre-set by tests
		go tx.readLoop(ctx)
		return nil
	}
	address := "192.168.4.1:1337"
	if strings.HasPrefix(tx.cfg.Port, "tcp://") {
		address = tx.cfg.Port[len("tcp://"):]
	}
	if !strings.HasSuffix(address, ":1337") {
		address += ":1337"
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	port, err := d.Dial("tcp", address)
	if err != nil {
		return err
	}
	if t, ok := port.(*net.TCPConn); ok {
		t.SetNoDelay(true) // low latency for small log messages
	}
	tx.port = port

	tx.port.Write([]byte("ccc"))

	if minVersion := tx.cfg.Extra["minversion"]; minVersion != "" {
		if err := tx.checkVersion(minVersion); err != nil {
			tx.port.Close()
			return err
		}
	}

	canRate := uint16(tx.cfg.CANRate)
	if err := tx.Command('o', []byte{uint8(canRate), uint8(canRate >> 8)}); err != nil {
		tx.port.Close()
		return err
	}

	go tx.readLoop(ctx)
	return nil
}

func (tx *Txbridge) Close() error {
	if tx.port == nil {
		return nil
	}
	tx.port.Write([]byte("c")) // best-effort channel close
	err := tx.port.Close()
	tx.port = nil
	return err
}

// Send serializes and writes a single CAN frame; the write completing is the
// confirmation.
func (tx *Txbridge) Send(ctx context.Context, f gocan.Frame) error {
	return tx.Command('t', append([]byte{uint8(f.ID >> 8), uint8(f.ID), f.Length}, f.Bytes()...))
}

// Command frames and writes one serial command (cmd, len, data, checksum).
func (tx *Txbridge) Command(cmd byte, data []byte) error {
	buf, err := (&serialcommand.SerialCommand{Command: cmd, Data: data}).MarshalBinary()
	if err != nil {
		return err
	}
	return tx.Raw(buf)
}

// Raw writes bytes to the dongle as-is, for the unframed one-shot commands
// ("5"/"7"/"8" ECU select, "r"/"s" logger start/stop, "g" gather, ...).
func (tx *Txbridge) Raw(data []byte) error {
	tx.writeMu.Lock()
	defer tx.writeMu.Unlock()
	if tx.port == nil {
		return errors.New("txbridge port not open")
	}
	n, err := tx.port.Write(data)
	if tx.cfg.Debug {
		tx.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: fmt.Sprintf("tx>> % X (wrote %d/%d, err=%v)", data, n, len(data), err)})
	}
	return err
}

// Subscribe delivers dongle commands carrying one of the given command bytes
// until ctx is cancelled. Delivery is non-blocking; slow readers lose
// commands.
func (tx *Txbridge) Subscribe(ctx context.Context, cmds ...byte) <-chan *serialcommand.SerialCommand {
	sub := &commandSub{
		ctx:  ctx,
		cmds: make(map[byte]struct{}, len(cmds)),
		ch:   make(chan *serialcommand.SerialCommand, 16),
	}
	for _, c := range cmds {
		sub.cmds[c] = struct{}{}
	}
	tx.subMu.Lock()
	tx.subs[sub] = struct{}{}
	tx.subMu.Unlock()
	context.AfterFunc(ctx, func() {
		tx.subMu.Lock()
		delete(tx.subs, sub)
		tx.subMu.Unlock()
		close(sub.ch)
	})
	return sub.ch
}

// Request sends one framed command and waits for the first reply carrying one
// of the given command bytes. Bound it with a context deadline.
func (tx *Txbridge) Request(ctx context.Context, cmd byte, data []byte, reply ...byte) (*serialcommand.SerialCommand, error) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := tx.Subscribe(sctx, reply...)
	if err := tx.Command(cmd, data); err != nil {
		return nil, err
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return nil, gocan.ErrClosed
		}
		return r, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// SetFilter installs the dongle's dynamic whitelist: only listed IDs are
// forwarded over the link (KWP IDs the firmware handles itself always pass).
// An empty list is a no-op so the firmware keeps its boot default.
func (tx *Txbridge) SetFilter(filters []uint32) error {
	if len(filters) == 0 {
		return nil
	}
	data := make([]byte, 0, len(filters)*2)
	for _, id := range filters {
		data = append(data, byte(id), byte(id>>8)) // 11-bit IDs, little-endian
	}
	return tx.Command('f', data)
}

func (tx *Txbridge) dispatch(cmd *serialcommand.SerialCommand) {
	tx.subMu.Lock()
	defer tx.subMu.Unlock()
	for sub := range tx.subs {
		if sub.ctx.Err() != nil {
			continue
		}
		if _, ok := sub.cmds[cmd.Command]; !ok {
			continue
		}
		select {
		case sub.ch <- cmd:
		default:
		}
	}
}

func (tx *Txbridge) checkVersion(minVersion string) error {
	cmd := serialcommand.NewSerialCommand('v', []byte{0x10})
	buf, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}
	if _, err := tx.port.Write(buf); err != nil {
		return err
	}
	resp, err := readSerialCommand(tx.port, 5*time.Second)
	if err != nil {
		return err
	}
	if resp.Command == 'e' {
		return fmt.Errorf("version check failed: %X %X", resp.Command, resp.Data)
	}
	if resp.Command != 'v' {
		return fmt.Errorf("unexpected version response: %X %X", resp.Command, resp.Data)
	}
	tx.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "txbridge firmware version: " + string(resp.Data)})
	if semver.Compare("v"+string(resp.Data), "v"+minVersion) < 0 {
		return fmt.Errorf("txbridge firmware %s or newer is required (dongle has %s), please update the dongle", minVersion, string(resp.Data))
	}
	return nil
}

func (tx *Txbridge) readLoop(ctx context.Context) {
	var (
		parsingCommand  bool
		haveLength      bool // length byte read? zero is a legitimate size
		command         uint8
		commandSize     uint8
		commandChecksum uint8
		cmdbuffPtr      uint8
	)
	cmdbuff := make([]byte, 256)
	readbuf := make([]byte, 4096)

	reset := func() {
		parsingCommand = false
		haveLength = false
		commandSize = 0
		commandChecksum = 0
		cmdbuffPtr = 0
	}

	for {
		n, err := tx.port.Read(readbuf)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				tx.bus.Fatal(err)
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		for _, b := range readbuf[:n] {
			if !parsingCommand {
				switch b {
				case 'e', 't', 'r', 'R', 'w', 'W', 'G':
					parsingCommand = true
					haveLength = false
					command = b
					commandSize = 0
					commandChecksum = 0
					cmdbuffPtr = 0
				}
				continue
			}
			if !haveLength {
				commandSize = b
				haveLength = true
				continue
			}
			if cmdbuffPtr == commandSize {
				if commandChecksum != b {
					tx.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("checksum error: expected %02X, got %02X", commandChecksum, b)})
					reset()
					continue
				}
				data := make([]byte, commandSize)
				copy(data, cmdbuff[:cmdbuffPtr])
				tx.handleCommand(command, data)
				reset()
				continue
			}
			if cmdbuffPtr < commandSize {
				cmdbuff[cmdbuffPtr] = b
				cmdbuffPtr++
				commandChecksum += b
			}
		}
	}
}

func (tx *Txbridge) handleCommand(command byte, data []byte) {
	if tx.cfg.Debug {
		tx.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: fmt.Sprintf("rx<< %q % X", command, data)})
	}
	switch command {
	case 't', 'T':
		// Guard: the 1-byte sum checksum is weak enough that a corrupted
		// frame can pass with a short size; don't panic on data[0]/data[1].
		if len(data) < 2 || len(data) > 2+8 {
			tx.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("bad %q frame: %X", command, data)})
			return
		}
		f := gocan.Frame{
			ID:     uint32(data[0])<<8 | uint32(data[1]),
			Length: uint8(len(data) - 2),
		}
		copy(f.Data[:], data[2:])
		tx.bus.Deliver(f)
	case 'e':
		if len(data) < 2 {
			tx.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("short %q frame: %X", command, data)})
			return
		}
		switch data[1] {
		case 0x31:
			tx.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: "read timeout"})
		case 0x32:
			tx.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: "invalid sequence"})
		default:
			tx.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("Unknown: %X", data)})
		}
		tx.dispatch(&serialcommand.SerialCommand{Command: command, Data: data})
	default:
		// dongle host-side commands ('r', 'R', 'w', 'W', 'G', ...)
		tx.dispatch(&serialcommand.SerialCommand{Command: command, Data: data})
	}
}

// readSerialCommand reads a single framed command, for the pre-read-loop
// version probe.
func readSerialCommand(port io.Reader, timeout time.Duration) (*serialcommand.SerialCommand, error) {
	deadline := time.Now().Add(timeout)
	var (
		parsingCommand  bool
		haveLength      bool
		command         byte
		commandSize     byte
		commandChecksum byte
		cmdbuff         = make([]byte, 256)
		cmdbuffPtr      byte
	)
	readbuf := make([]byte, 16)
	for time.Now().Before(deadline) {
		n, err := port.Read(readbuf)
		if err != nil {
			return nil, fmt.Errorf("read error: %w", err)
		}
		for _, b := range readbuf[:n] {
			if !parsingCommand {
				parsingCommand = true
				command = b
				continue
			}
			if !haveLength {
				commandSize = b
				haveLength = true
				continue
			}
			if cmdbuffPtr == commandSize {
				if commandChecksum != b {
					return nil, fmt.Errorf("checksum error: expected %02X, got %02X", b, commandChecksum)
				}
				return &serialcommand.SerialCommand{
					Command: command,
					Data:    append([]byte(nil), cmdbuff[:cmdbuffPtr]...),
				}, nil
			}
			if cmdbuffPtr < commandSize {
				cmdbuff[cmdbuffPtr] = b
				cmdbuffPtr++
				commandChecksum += b
			}
		}
	}
	return nil, fmt.Errorf("timeout after %v", timeout)
}
