package txbridge

import (
	"context"
	"net"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
)

// dongleFrame builds the dongle->host encoding of a received CAN frame:
// framed 't' command with payload [idHi, idLo, data...] (no DLC byte).
func dongleFrame(t *testing.T, id uint16, data []byte) []byte {
	t.Helper()
	cmd := &serialcommand.SerialCommand{
		Command: 't',
		Data:    append([]byte{byte(id >> 8), byte(id)}, data...),
	}
	buf, err := cmd.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

// TestKWPExchange replays the T7 StartSession exchange: host sends a frame
// on 0x220, the fake dongle replies on 0x238, and bus.Request must return
// the reply.
func TestKWPExchange(t *testing.T) {
	host, dongle := net.Pipe()
	tx := &Txbridge{port: host, subs: make(map[*commandSub]struct{})}

	bus, err := gocan.OpenAdapter(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	// fake dongle: read the framed 't' command, verify, answer on 0x238,
	// then keep draining so teardown writes (Close's "c") never block the
	// unbuffered pipe.
	go func() {
		defer func() {
			buf := make([]byte, 64)
			for {
				if _, err := dongle.Read(buf); err != nil {
					return
				}
			}
		}()
		buf := make([]byte, 64)
		n, err := dongle.Read(buf)
		if err != nil {
			t.Error(err)
			return
		}
		cmd := &serialcommand.SerialCommand{}
		if err := cmd.UnmarshalBinary(buf[:n]); err != nil {
			t.Errorf("dongle could not parse host frame: %v (% X)", err, buf[:n])
			return
		}
		if cmd.Command != 't' {
			t.Errorf("expected 't', got %q", cmd.Command)
			return
		}
		wantTX := []byte{0x02, 0x20, 6, 0x3F, 0x81, 0x00, 0x11, 0x02, 0x40}
		if len(cmd.Data) != len(wantTX) {
			t.Errorf("tx payload = % X, want % X", cmd.Data, wantTX)
			return
		}
		for i := range wantTX {
			if cmd.Data[i] != wantTX[i] {
				t.Errorf("tx payload = % X, want % X", cmd.Data, wantTX)
				return
			}
		}
		// ECU reply: 0x238 with 8 data bytes
		dongle.Write(dongleFrame(t, 0x238, []byte{0x40, 0xBF, 0x21, 0xC1, 0x00, 0x11, 0x02, 0x58}))
	}()

	rctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := bus.Request(rctx, gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40}), 0x238)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.ID != 0x238 || resp.Length != 8 || resp.Data[3] != 0xC1 {
		t.Fatalf("bad reply: %s", resp.String())
	}
}

// TestByteAtATime feeds the reply one byte at a time to exercise the parser
// state machine across read boundaries.
func TestByteAtATime(t *testing.T) {
	host, dongle := net.Pipe()
	tx := &Txbridge{port: host, subs: make(map[*commandSub]struct{})}
	bus, err := gocan.OpenAdapter(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	ch := bus.Subscribe(context.Background(), 0x258)
	go func() {
		frame := dongleFrame(t, 0x258, []byte{0xC0, 0xBF, 0x02, 0xC1, 0x00, 0x00, 0x00, 0x00})
		for _, b := range frame {
			dongle.Write([]byte{b})
		}
		buf := make([]byte, 64)
		for {
			if _, err := dongle.Read(buf); err != nil {
				return
			}
		}
	}()
	select {
	case f := <-ch:
		if f.ID != 0x258 || f.Length != 8 {
			t.Fatalf("bad frame: %s", f.String())
		}
	case <-time.After(time.Second):
		t.Fatal("frame never delivered")
	}
}
