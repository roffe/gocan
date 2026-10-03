//go:build rcan

package rcan

import (
	"context"
	"encoding/hex"
	"testing"

	gocan "github.com/roffe/gocan/v2"
)

func TestRCanEncode(t *testing.T) {
	var buf [4 + 8]byte
	for _, tt := range []struct {
		f    gocan.Frame
		want string
	}{
		{gocan.NewFrame(0x220, []byte{0x3F, 0x81}), "050220023f81"},
		{gocan.NewFrame(0x7E0, []byte{1, 2, 3, 4, 5, 6, 7, 8}), "0507e0080102030405060708"},
		{gocan.NewFrame(0x123, nil), "05012300"},
	} {
		if got := hex.EncodeToString(encode(buf[:0], tt.f)); got != tt.want {
			t.Errorf("encode(%s) = %s, want %s", tt.f, got, tt.want)
		}
	}
	f := gocan.NewFrame(0x7E0, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if n := testing.AllocsPerRun(100, func() { encode(buf[:0], f) }); n != 0 {
		t.Fatalf("encode allocates %v times per frame, want 0", n)
	}
}

func BenchmarkRCanEncode(b *testing.B) {
	f := gocan.NewFrame(0x7E0, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	var buf [4 + 8]byte
	b.ReportAllocs()
	for b.Loop() {
		encode(buf[:0], f)
	}
}

func BenchmarkRCanDeliver(b *testing.B) {
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		b.Fatal(err)
	}
	defer bus.Close()
	r := &RCan{bus: bus}
	pkt := []byte{cmdCANFrame, 0x8<<4 | 0x2, 0x58, 1, 2, 3, 4, 5, 6, 7, 8} // dlc<<4|idHi, idLo, data
	b.ReportAllocs()
	for b.Loop() {
		r.deliver(pkt)
	}
}
