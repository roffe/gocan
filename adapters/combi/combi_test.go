package combi

import (
	"context"
	"encoding/hex"
	"testing"

	gocan "github.com/roffe/gocan/v2"
)

func TestCombiEncode(t *testing.T) {
	ext := gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	ext.Remote = true
	var buf [19]byte
	for _, tt := range []struct {
		f    gocan.Frame
		want string
	}{
		{gocan.NewFrame(0x220, []byte{0x3F, 0x81}), "83000f200200003f8100000000000002000000"},
		{ext, "83000f10f1da18010203040506070808010100"},
		{gocan.NewFrame(0x7E0, nil), "83000fe0070000000000000000000000000000"}, // ext/rtr must not leak from the previous frame
	} {
		if encode(&buf, tt.f); hex.EncodeToString(buf[:]) != tt.want {
			t.Errorf("encode(%s) = %x, want %s", tt.f, buf, tt.want)
		}
	}
	if n := testing.AllocsPerRun(100, func() { encode(&buf, ext) }); n != 0 {
		t.Fatalf("encode allocates %v times per frame, want 0", n)
	}
}

func BenchmarkCombiEncode(b *testing.B) {
	f := gocan.NewFrame(0x220, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	var buf [19]byte
	b.ReportAllocs()
	for b.Loop() {
		encode(&buf, f)
	}
}

func BenchmarkCombiDeliver(b *testing.B) {
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		b.Fatal(err)
	}
	defer bus.Close()
	ca := &Combi{bus: bus}
	payload := []byte{0x58, 0x02, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 8, 0, 0} // ID LE, data, DLC, ext, rtr
	b.ReportAllocs()
	for b.Loop() {
		ca.deliverFrame(payload)
	}
}
