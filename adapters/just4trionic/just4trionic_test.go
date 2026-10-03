package just4trionic

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

func TestJust4TrionicSendEncoding(t *testing.T) {
	fp := &fakePort{}
	a := &Just4Trionic{port: fp}
	for _, tt := range []struct {
		f    gocan.Frame
		want string
	}{
		{gocan.NewFrame(0x7, nil), "t700000000000000000\r"},
		{gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0xAB}), "t22033f81ab0000000000\r"},
		{gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3, 4, 5, 6, 7, 0xFE}), "t18daf110801020304050607fe\r"},
	} {
		if err := a.Send(context.Background(), tt.f); err != nil {
			t.Fatal(err)
		}
		if got := string(fp.last); got != tt.want {
			t.Errorf("Send(%s) wrote %q, want %q", tt.f, got, tt.want)
		}
	}
	f := gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0xAB})
	if n := testing.AllocsPerRun(100, func() { a.Send(context.Background(), f) }); n != 0 {
		t.Fatalf("Send allocates %v times per frame, want 0", n)
	}
}

func TestJust4TrionicParse(t *testing.T) {
	bus := newBus(t)
	frames := bus.SubscribeN(context.Background(), 4)
	a := &Just4Trionic{bus: bus}
	a.parse([]byte("\r\nw25883F81112233445566\r\n"))
	if got, want := <-frames, gocan.NewFrame(0x258, []byte{0x3F, 0x81, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66}); got != want {
		t.Fatalf("decoded %s, want %s", got, want)
	}
	a.parse([]byte("w\n"))            // noise: reported as a short frame, must not panic
	a = &Just4Trionic{bus: newBus(t)} // no subscribers: Deliver itself must not allocate
	data := []byte("w25883F81112233445566\r\n")
	if n := testing.AllocsPerRun(100, func() { a.parse(data) }); n != 0 {
		t.Fatalf("parse allocates %v times per frame, want 0", n)
	}
}

func BenchmarkJust4TrionicSend(b *testing.B) {
	a := &Just4Trionic{port: &fakePort{}}
	f := gocan.NewFrame(0x220, []byte{0x3F, 0x81, 0xAB})
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		a.Send(ctx, f)
	}
}

func BenchmarkJust4TrionicParse(b *testing.B) {
	a := &Just4Trionic{bus: newBus(b)}
	data := []byte("w25883F81112233445566\r\n")
	b.ReportAllocs()
	for b.Loop() {
		a.parse(data)
	}
}
