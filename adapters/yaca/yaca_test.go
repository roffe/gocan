package yaca

import (
	"context"
	"testing"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

// writePort records the last Write; Send uses nothing else of serial.Port.
type writePort struct {
	serial.Port
	wrote []byte
}

func (p *writePort) Write(b []byte) (int, error) {
	p.wrote = append(p.wrote[:0], b...)
	return len(b), nil
}

func TestYACASend(t *testing.T) {
	p := &writePort{}
	ya := &YACA{port: p}
	ctx := context.Background()
	for _, tt := range []struct {
		f    gocan.Frame
		want string
	}{
		{gocan.NewFrame(0x7, nil), "t0070\r"},
		{gocan.NewFrame(0x240, []byte{0x3F, 0x81}), "t24023f81\r"},
		{gocan.NewFrame(0xFFFF, []byte{1, 2, 3, 4, 5, 6, 7, 8}), "tfff80102030405060708\r"}, // masked to 12 bits
		{gocan.NewExtendedFrame(0x18DAF110, []byte{0xAB}), "t1101ab\r"},                     // no extended support
	} {
		if err := ya.Send(ctx, tt.f); err != nil {
			t.Fatal(err)
		}
		if got := string(p.wrote); got != tt.want {
			t.Errorf("Send(%s) wrote %q, want %q", tt.f, got, tt.want)
		}
	}
	f := gocan.NewFrame(0x258, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if n := testing.AllocsPerRun(100, func() { ya.Send(ctx, f) }); n != 0 {
		t.Fatalf("Send allocates %v times per frame, want 0", n)
	}
}

func BenchmarkYACASend(b *testing.B) {
	ya := &YACA{port: &writePort{}}
	ctx := context.Background()
	f := gocan.NewFrame(0x258, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	b.ReportAllocs()
	for b.Loop() {
		ya.Send(ctx, f)
	}
}

func BenchmarkYACAParse(b *testing.B) {
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		b.Fatal(err)
	}
	defer bus.Close()
	ya := &YACA{bus: bus}
	data := []byte("t25883f81112233445566\n")
	b.ReportAllocs()
	for b.Loop() {
		ya.parse(data)
	}
}
