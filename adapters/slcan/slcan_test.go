package slcan

import (
	"context"
	"testing"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

// fakePort records the last write; the embedded nil serial.Port panics on
// anything else.
type fakePort struct {
	serial.Port
	last []byte
}

func (p *fakePort) Write(b []byte) (int, error) {
	p.last = append(p.last[:0], b...)
	return len(b), nil
}

func newBus(tb testing.TB) *gocan.Bus {
	tb.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { bus.Close() })
	return bus
}

func TestSLCanSendEncoding(t *testing.T) {
	fp := &fakePort{}
	sl := &SLCan{port: fp}
	for _, tt := range []struct {
		f    gocan.Frame
		want string
	}{
		{gocan.NewFrame(0x7, nil), "t0070\r"},
		{gocan.NewFrame(0xFFFF, []byte{0xAB}), "t7FF1AB\r"}, // masked to 11 bits
		{gocan.NewFrame(0x240, []byte{0x3F, 0x81, 1, 2, 3, 4, 5, 0xFE}), "t24083F810102030405FE\r"},
	} {
		if err := sl.Send(context.Background(), tt.f); err != nil {
			t.Fatal(err)
		}
		if got := string(fp.last); got != tt.want {
			t.Errorf("Send(%s) wrote %q, want %q", tt.f, got, tt.want)
		}
	}
	f := gocan.NewFrame(0x240, []byte{0x3F, 0x81, 1, 2, 3, 4, 5, 6})
	if n := testing.AllocsPerRun(100, func() { sl.Send(context.Background(), f) }); n != 0 {
		t.Fatalf("Send allocates %v times per frame, want 0", n)
	}
}

func TestSLCanParse(t *testing.T) {
	bus := newBus(t)
	frames := bus.SubscribeN(context.Background(), 4)
	sl := &SLCan{bus: bus}
	sl.parse([]byte("z\rt25883F81112233445566\r"))
	if got, want := <-frames, gocan.NewFrame(0x258, []byte{0x3F, 0x81, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66}); got != want {
		t.Fatalf("decoded %s, want %s", got, want)
	}
	sl = &SLCan{bus: newBus(t)} // no subscribers: Deliver itself must not allocate
	data := []byte("t25883F81112233445566\rz\r")
	if n := testing.AllocsPerRun(100, func() { sl.parse(data) }); n != 0 {
		t.Fatalf("parse allocates %v times per frame, want 0", n)
	}
}

func BenchmarkSLCanSend(b *testing.B) {
	sl := &SLCan{port: &fakePort{}}
	f := gocan.NewFrame(0x240, []byte{0x3F, 0x81, 1, 2, 3, 4, 5, 6})
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		sl.Send(ctx, f)
	}
}

func BenchmarkSLCanParse(b *testing.B) {
	sl := &SLCan{bus: newBus(b)}
	data := []byte("t25883F81112233445566\rz\r")
	b.ReportAllocs()
	for b.Loop() {
		sl.parse(data)
	}
}
