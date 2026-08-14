// Package obdx drives the OBDX Pro Wifi over its TCP AP (192.168.4.1:23)
// using the binary DVI command protocol (framing reused from gocan
// v1's pkg/dvi). Importing the package registers the "OBDX Pro Wifi"
// adapter.
//
// The device is switched from AT to DVI mode with DXDP1, then configured
// for raw HS-CAN at 500 kbit/s (the bit-rate is fixed by the init sequence,
// matching the v1 adapter) with automatic frame processing, padding and
// write-response status disabled. Received network frames arrive as DVI
// command 0x08 with a big-endian ID followed by the payload.
package obdx

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/dvi"
)

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name:         "OBDX Pro Wifi",
		Description:  "OBDX Pro Wifi",
		Capabilities: gocan.Capabilities{HSCAN: true, SWCAN: true, KLine: true},
		New:          New,
	})
}

type OBDXProWifi struct {
	cfg       gocan.Config
	bus       *gocan.Bus
	conn      net.Conn
	closeOnce sync.Once
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &OBDXProWifi{cfg: cfg}, nil
}

func (a *OBDXProWifi) Open(ctx context.Context, bus *gocan.Bus) error {
	a.bus = bus
	d := net.Dialer{Timeout: 5 * time.Second}
	var err error
	a.conn, err = d.Dial("tcp", "192.168.4.1:23")
	if err != nil {
		return err
	}

	a.conn.Write([]byte("ATAR\r"))
	time.Sleep(20 * time.Millisecond)
	a.conn.Write([]byte("DXDP1\r")) // switch to DVI protocol mode
	time.Sleep(50 * time.Millisecond)

	// Drain whatever the AT prompt left in the pipe before parsing DVI.
	slask := make([]byte, 1024)
	if _, err := a.conn.Read(slask); err != nil {
		a.conn.Close()
		return err
	}

	initCommands := []*dvi.Command{
		dvi.New(0x31, []byte{0x01, 0x02}), // set HS CAN
		dvi.New(0x34, []byte{0x15, 0x06}), // set CAN speed to 500kbit
		dvi.New(0x34, []byte{0x0F, 0x00}), // disable automatic formatting for writing network frames
		dvi.New(0x34, []byte{0x0B, 0x00}), // disable automatic frame processing for received network messages
		dvi.New(0x34, []byte{0x0E, 0x00}), // disable padding
		dvi.New(0x24, []byte{0x01, 0x00}), // disable network write responses status
	}
	for _, cmd := range initCommands {
		if _, err := a.conn.Write(cmd.Bytes()); err != nil {
			a.conn.Close()
			return fmt.Errorf("failed to send init command: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	for i, id := range a.cfg.CANFilter {
		filterCMD := dvi.New(0x34, []byte{0x00, byte(i), dvi.FRAME_TYPE_11BIT, dvi.FILTER_TYPE_PASS, dvi.FILTER_STATUS_ON, byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id), 0x00, 0x00, 0x07, 0xFF, 0x00, 0x00, 0x00, 0x00})
		if _, err := a.conn.Write(filterCMD.Bytes()); err != nil {
			a.conn.Close()
			return fmt.Errorf("failed to set filter: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// enable networking
	if _, err := a.conn.Write(dvi.New(0x31, []byte{0x02, 0x01}).Bytes()); err != nil {
		a.conn.Close()
		return fmt.Errorf("failed to enable networking: %w", err)
	}

	go a.readLoop(ctx)
	return nil
}

func (a *OBDXProWifi) Close() error {
	a.closeOnce.Do(func() {
		if a.conn == nil {
			return
		}
		time.Sleep(80 * time.Millisecond)
		cmds := []*dvi.Command{
			dvi.New(0x31, []byte{0x02, 0x00}), // disable networking
			dvi.New(0x25, []byte{}),           // reset
		}
		for _, cmd := range cmds {
			a.conn.Write(cmd.Bytes()) // best-effort teardown
			time.Sleep(20 * time.Millisecond)
		}
		a.conn.Close()
	})
	return nil
}

func (a *OBDXProWifi) Send(ctx context.Context, f gocan.Frame) error {
	sendCmd := dvi.New(dvi.CMD_SEND_TO_NETWORK_NORMAL,
		append([]byte{byte(f.ID >> 24), byte(f.ID >> 16), byte(f.ID >> 8), byte(f.ID)}, f.Bytes()...))
	if a.cfg.Debug {
		a.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "dvi out: " + sendCmd.String()})
	}
	if _, err := a.conn.Write(sendCmd.Bytes()); err != nil {
		return fmt.Errorf("failed to send frame: %w", err)
	}
	return nil
}

func (a *OBDXProWifi) readLoop(ctx context.Context) {
	parser := dvi.NewCommandParser(func(cmd *dvi.Command) {
		if a.cfg.Debug {
			a.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: "dvi in: " + cmd.String()})
		}
		if cmd.Command() != 0x08 { // network frame
			return
		}
		data := cmd.Data()
		if len(data) < 4 || len(data) > 12 {
			a.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: fmt.Sprintf("bad dvi frame: % 02X", data)})
			return
		}
		f := gocan.Frame{
			ID:     binary.BigEndian.Uint32(data[:4]),
			Length: uint8(len(data) - 4),
		}
		copy(f.Data[:], data[4:])
		a.bus.Deliver(f)
	})
	buf := make([]byte, 16)
	for {
		n, err := a.conn.Read(buf)
		if err != nil {
			if ctx.Err() == nil {
				a.bus.Fatal(fmt.Errorf("failed to read from OBDX: %w", err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		parser.AddData(buf[:n])
	}
}
