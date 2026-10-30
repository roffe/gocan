//go:build rcan

package rcan

import (
	"context"
	"encoding/hex"
	"slices"
	"testing"

	gocan "github.com/roffe/gocan/v2"
)

// rxFrame is the packet the firmware sends for a standard frame:
// cmd, dlc<<4|idHi, idLo, data.
func rxFrame(f gocan.Frame) []byte {
	return append([]byte{cmdCANFrame, f.Length<<4 | byte(f.ID>>8)&0x0F, byte(f.ID)}, f.Data[:f.Length]...)
}

func TestRCanParse(t *testing.T) {
	full := gocan.NewFrame(0x258, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	empty := gocan.NewFrame(0x7FF, nil)
	for _, tt := range []struct {
		name   string
		in     []byte
		frames []gocan.Frame
		errs   int
	}{
		{"frame", rxFrame(full), []gocan.Frame{full}, 0},
		{"wire bytes", []byte{0x05, 0x22, 0x20, 0x3F, 0x81}, []gocan.Frame{gocan.NewFrame(0x220, []byte{0x3F, 0x81})}, 0},
		{"no data", rxFrame(empty), []gocan.Frame{empty}, 0},
		{"two frames", slices.Concat(rxFrame(empty), rxFrame(full)), []gocan.Frame{empty, full}, 0},
		{"garbage first", slices.Concat([]byte{0x55, 0x66}, rxFrame(full)), []gocan.Frame{full}, 0},
		{"invalid dlc", slices.Concat([]byte{cmdCANFrame, 0x92, 0x58}, rxFrame(full)), []gocan.Frame{full}, 1},
		{"id past 11 bits", slices.Concat([]byte{cmdCANFrame, 0x38, 0x30, 1, 2, 3}, rxFrame(full)), []gocan.Frame{full}, 1},
		{"incomplete", rxFrame(full)[:7], nil, 0},
	} {
		for split := range len(tt.in) {
			bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
			if err != nil {
				t.Fatal(err)
			}
			var errs int
			bus.OnEvent(func(e gocan.Event) {
				if e.Type == gocan.EventTypeError {
					errs++
				}
			})
			frames := bus.SubscribeN(context.Background(), 64)
			r := &RCan{bus: bus}
			r.parse(tt.in[:split])
			r.parse(tt.in[split:])
			var got []gocan.Frame
			for len(frames) > 0 {
				got = append(got, <-frames)
			}
			bus.Close()
			if !slices.Equal(got, tt.frames) || errs != tt.errs {
				t.Errorf("%s split at %d: frames %v, %d errors; want %v, %d errors", tt.name, split, got, errs, tt.frames, tt.errs)
			}
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add(rxFrame(gocan.NewFrame(0x258, []byte{1, 2, 3, 4, 5, 6, 7, 8})))
	f.Add([]byte{0x55, cmdCANFrame, 0x92, 0x58, cmdCANFrame, 0x07, 0xE8})
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { bus.Close() })
	frames := bus.SubscribeN(context.Background(), 4096)
	f.Fuzz(func(t *testing.T, data []byte) {
		(&RCan{bus: bus}).parse(data)
		for len(frames) > 0 {
			if fr := <-frames; fr.Length > 8 || fr.ID > 0x7FF {
				t.Fatalf("frame %s from % X", fr, data)
			}
		}
	})
}

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
