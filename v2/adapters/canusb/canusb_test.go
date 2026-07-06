package canusb

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

// fakePort implements serialPort with serial-like read-timeout semantics:
// Read returns (0, nil) when no data arrives in time, an error once closed.
type fakePort struct {
	mu      sync.Mutex
	wrote   []string
	rx      chan byte
	closeCh chan struct{}
	once    sync.Once
	autoAck bool // reply z/Z to every transmit command
}

func newFakePort(autoAck bool) *fakePort {
	return &fakePort{rx: make(chan byte, 1024), closeCh: make(chan struct{}), autoAck: autoAck}
}

func (p *fakePort) feed(s string) {
	for _, b := range []byte(s) {
		p.rx <- b
	}
}

func (p *fakePort) writes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.wrote...)
}

func (p *fakePort) Write(b []byte) (int, error) {
	select {
	case <-p.closeCh:
		return 0, io.ErrClosedPipe
	default:
	}
	p.mu.Lock()
	p.wrote = append(p.wrote, string(b))
	p.mu.Unlock()
	if p.autoAck {
		switch b[0] {
		case 't':
			p.feed("z\r")
		case 'T':
			p.feed("Z\r")
		}
	}
	return len(b), nil
}

func (p *fakePort) Read(b []byte) (int, error) {
	select {
	case <-p.closeCh:
		return 0, io.ErrClosedPipe
	case c := <-p.rx:
		b[0] = c
		n := 1
		for n < len(b) {
			select {
			case c := <-p.rx:
				b[n] = c
				n++
			default:
				return n, nil
			}
		}
		return n, nil
	case <-time.After(time.Millisecond):
		return 0, nil
	}
}

func (p *fakePort) Close() error {
	p.once.Do(func() { close(p.closeCh) })
	return nil
}

func (p *fakePort) SetReadTimeout(time.Duration) error { return nil }
func (p *fakePort) ResetInputBuffer() error            { return nil }
func (p *fakePort) ResetOutputBuffer() error           { return nil }

func openCANUSB(t *testing.T, fp *fakePort, opts ...gocan.Option) *gocan.Bus {
	t.Helper()
	a, err := New(gocan.Config{CANRate: 500, CANFilter: []uint32{0x238, 0x258}})
	if err != nil {
		t.Fatal(err)
	}
	a.(*CANUSB).port = fp
	bus, err := gocan.OpenAdapter(context.Background(), a, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	return bus
}

func TestCANUSBOpenSequence(t *testing.T) {
	fp := newFakePort(true)
	bus := openCANUSB(t, fp)

	want := []string{"\r", "\r", "\r", "V\r", "N\r", "Z0\r", "S6\r", "M00004300\r", "m00000C10\r", "O\r"}
	got := fp.writes()
	if len(got) < len(want) {
		t.Fatalf("want %d setup writes, got %v", len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("setup write %d: want %q, got %q", i, w, got[i])
		}
	}

	bus.Close()
	last := fp.writes()
	if last[len(last)-1] != "C\r" {
		t.Fatalf("close should send C, got %q", last[len(last)-1])
	}
	if err := bus.Err(); err != nil {
		t.Fatalf("clean close should report nil, got %v", err)
	}
}

func TestCANUSBSendReceive(t *testing.T) {
	fp := newFakePort(true)
	bus := openCANUSB(t, fp)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sub := bus.Subscribe(ctx, 0x258)

	if err := bus.Send(ctx, gocan.NewFrame(0x240, []byte{0x3F, 0x81})); err != nil {
		t.Fatal(err)
	}
	w := fp.writes()
	if w[len(w)-1] != "t24023f81\r" {
		t.Fatalf("unexpected transmit encoding: %q", w[len(w)-1])
	}

	if err := bus.Send(ctx, gocan.NewExtendedFrame(0x18DAF110, []byte{1})); err != nil {
		t.Fatal(err)
	}
	w = fp.writes()
	if w[len(w)-1] != "T18DAF110101\r" {
		t.Fatalf("unexpected extended transmit encoding: %q", w[len(w)-1])
	}

	fp.feed("t25883F81112233445566\r")
	select {
	case f := <-sub:
		if f.ID != 0x258 || f.Extended || f.Length != 8 || f.Data != [8]byte{0x3F, 0x81, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66} {
			t.Fatalf("bad decoded frame: %s", f)
		}
	case <-ctx.Done():
		t.Fatal("no frame delivered")
	}
}

func TestCANUSBSendGatedOnAck(t *testing.T) {
	fp := newFakePort(false) // no auto-ack
	bus := openCANUSB(t, fp)

	if err := bus.Send(context.Background(), gocan.NewFrame(0x123, nil)); err != nil {
		t.Fatal(err)
	}
	// Second send must block until the device acks the first.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := bus.Send(ctx, gocan.NewFrame(0x124, nil)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded while unacked, got %v", err)
	}
	fp.feed("z\r")
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := bus.Send(ctx2, gocan.NewFrame(0x124, nil)); err != nil {
		t.Fatalf("send after ack: %v", err)
	}
}

func TestCANUSBErrorReplies(t *testing.T) {
	fp := newFakePort(true)
	var mu sync.Mutex
	var events []gocan.Event
	openCANUSB(t, fp, gocan.WithEventFunc(func(e gocan.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}))

	fp.feed("\a")    // BELL: command error
	fp.feed("F04\r") // status: error warning (EI)

	deadline := time.After(time.Second)
	for {
		mu.Lock()
		var gotBell, gotStatus bool
		for _, e := range events {
			if e.Type == gocan.EventTypeError {
				gotBell = gotBell || strings.Contains(e.Details, "BELL")
				gotStatus = gotStatus || strings.Contains(e.Details, "error warning")
			}
		}
		mu.Unlock()
		if gotBell && gotStatus {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("missing error events, got %v", events)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestCANUSBAcceptanceFilters(t *testing.T) {
	code, mask := acceptanceFilters([]uint32{0x238, 0x258})
	if code != "M00004300" || mask != "m00000C10" {
		t.Fatalf("got %s %s", code, mask)
	}
	// Unfilterable input falls back to accept-everything.
	code, mask = acceptanceFilters(nil)
	if code != "M00000000" || mask != "mFFFFFFFF" {
		t.Fatalf("fallback: got %s %s", code, mask)
	}
}
