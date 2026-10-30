package combi

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"math"
	"slices"
	"testing"

	gocan "github.com/roffe/gocan/v2"
)

func newTestCombi(tb testing.TB) (ca *Combi, frames <-chan gocan.Frame, errs *[]string) {
	tb.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { bus.Close() })
	errs = new([]string)
	bus.OnEvent(func(e gocan.Event) {
		if e.Type == gocan.EventTypeError {
			*errs = append(*errs, e.Details)
		}
	})
	a, err := New(gocan.Config{})
	if err != nil {
		tb.Fatal(err)
	}
	ca = a.(*Combi)
	ca.bus = bus
	return ca, bus.SubscribeN(context.Background(), 64), errs
}

func drain(ch <-chan gocan.Frame) []gocan.Frame {
	var out []gocan.Frame
	for len(ch) > 0 {
		out = append(out, <-ch)
	}
	return out
}

// rxFrame is the cmdCanFrame packet the firmware sends for f: the TX layout
// pinned by TestCombiEncode under the RX command byte.
func rxFrame(f gocan.Frame) []byte {
	var b [19]byte
	encode(&b, f)
	b[0] = cmdCanFrame
	return b[:]
}

func TestCombiParse(t *testing.T) {
	std := gocan.NewFrame(0x220, []byte{0x3F, 0x81})
	ext := gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	ext.Remote = true
	badTerm := rxFrame(std)
	badTerm[18] = 0x55
	nak := rxFrame(std)
	nak[18] = termNak
	longDLC := rxFrame(ext)
	longDLC[15] = 15
	wire, _ := hex.DecodeString("82000f200200003f8100000000000002000000")
	for _, tt := range []struct {
		name   string
		in     []byte
		frames []gocan.Frame
		errs   int
	}{
		{"frame", rxFrame(std), []gocan.Frame{std}, 0},
		{"wire bytes", wire, []gocan.Frame{std}, 0},
		{"two frames", slices.Concat(rxFrame(std), rxFrame(ext)), []gocan.Frame{std, ext}, 0},
		{"garbage first", slices.Concat([]byte{0x55}, rxFrame(std)), []gocan.Frame{std}, 1},
		{"oversized", slices.Concat([]byte{cmdCanFrame, 0x04, 0x00}, rxFrame(std)), []gocan.Frame{std}, 1},
		{"bad terminator", slices.Concat(badTerm, rxFrame(ext)), []gocan.Frame{ext}, 1},
		{"nak", nak, nil, 1},
		{"short payload", []byte{cmdCanFrame, 0x00, 0x03, 1, 2, 3, termAck}, nil, 1},
		{"dlc clamped", longDLC, []gocan.Frame{ext}, 0},
		{"board reply nobody waits for", []byte{cmdBrdFWVersion, 0x00, 0x02, 3, 1, termAck}, nil, 0},
	} {
		for split := range len(tt.in) {
			ca, frames, errs := newTestCombi(t)
			ca.parse(tt.in[:split])
			ca.parse(tt.in[split:])
			if got := drain(frames); !slices.Equal(got, tt.frames) || len(*errs) != tt.errs {
				t.Errorf("%s split at %d: frames %v errors %q, want %v and %d errors", tt.name, split, got, *errs, tt.frames, tt.errs)
			}
		}
	}
}

func TestCombiParseReplies(t *testing.T) {
	ca, _, errs := newTestCombi(t)

	ca.parse([]byte{cmdCanTxFrame, 0x00, 0x00, termAck})
	if got := recv(t, ca.txAck); !got {
		t.Error("TX ack read as NAK")
	}
	ca.parse([]byte{cmdCanTxFrame, 0x00, 0x00, termNak})
	if got := recv(t, ca.txAck); got {
		t.Error("TX NAK read as ack")
	}
	ca.parse([]byte{cmdCanFilter, 0x00, 0x00, termAck})
	recv(t, ca.filterAck)
	ca.parse(slices.Concat([]byte{cmdBrdADC, 0x00, 0x04}, binary.LittleEndian.AppendUint32(nil, math.Float32bits(12.5)), []byte{termAck}))
	if got := recv(t, ca.adcValue); got != 12.5 {
		t.Errorf("ADC value %v, want 12.5", got)
	}
	if len(*errs) != 0 {
		t.Errorf("errors %q", *errs)
	}
}

func recv[T any](t *testing.T, ch <-chan T) (v T) {
	t.Helper()
	select {
	case v = <-ch:
	default:
		t.Fatal("no reply delivered")
	}
	return v
}

func FuzzParse(f *testing.F) {
	f.Add(rxFrame(gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3, 4, 5, 6, 7, 8})))
	f.Add([]byte{cmdCanTxFrame, 0x00, 0x00, termNak, cmdBrdADC, 0x00, 0x04, 0, 0, 0x48, 0x41, termAck})
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { bus.Close() })
	frames := bus.SubscribeN(context.Background(), 4096)
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := New(gocan.Config{})
		if err != nil {
			t.Fatal(err)
		}
		ca := a.(*Combi)
		ca.bus = bus
		ca.parse(data)
		for len(frames) > 0 {
			if fr := <-frames; fr.Length > 8 {
				t.Fatalf("frame with %d data bytes from % X", fr.Length, data)
			}
		}
	})
}

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
