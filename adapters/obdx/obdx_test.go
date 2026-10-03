package obdx

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/dvi"
)

// fakeConn records the last write and streams rx cyclically to Read until
// reads run out, then cancels the read loop's ctx and returns EOF. The
// embedded nil net.Conn panics on anything else.
type fakeConn struct {
	net.Conn
	last   []byte
	rx     []byte
	pos    int
	reads  int
	cancel context.CancelFunc
}

func (c *fakeConn) Write(b []byte) (int, error) {
	c.last = append(c.last[:0], b...)
	return len(b), nil
}

func (c *fakeConn) Read(b []byte) (int, error) {
	if c.reads == 0 {
		c.cancel()
		return 0, io.EOF
	}
	c.reads--
	n := copy(b, c.rx[c.pos:])
	c.pos = (c.pos + n) % len(c.rx)
	return n, nil
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

// rxFrame is a DVI 0x08 network frame for 0x7E8 carrying 8 data bytes.
var rxFrame = dvi.New(0x08, []byte{0, 0, 0x07, 0xE8, 0x10, 0x14, 0x5A, 0x90, 0x59, 0x53, 0x33, 0x45}).Bytes()

func TestOBDXSendEncoding(t *testing.T) {
	fc := &fakeConn{}
	a := &OBDXProWifi{conn: fc}
	for _, tt := range []struct {
		f    gocan.Frame
		want []byte
	}{
		{gocan.NewFrame(0x7, nil), []byte{0x10, 0x04, 0, 0, 0, 0x07, 0xE4}},
		{gocan.NewFrame(0x7E0, []byte{0x02, 0x10, 0x81}), []byte{0x10, 0x07, 0, 0, 0x07, 0xE0, 0x02, 0x10, 0x81, 0x6E}},
		{gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3, 4, 5, 6, 7, 8}), []byte{0x10, 0x0C, 0x18, 0xDA, 0xF1, 0x10, 1, 2, 3, 4, 5, 6, 7, 8, 0xCC}},
	} {
		if err := a.Send(context.Background(), tt.f); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fc.last, tt.want) {
			t.Errorf("Send(%s) wrote % X, want % X", tt.f, fc.last, tt.want)
		}
	}
	f := gocan.NewFrame(0x7E0, []byte{0x02, 0x10, 0x81})
	if n := testing.AllocsPerRun(100, func() { a.Send(context.Background(), f) }); n != 0 {
		t.Fatalf("Send allocates %v times per frame, want 0", n)
	}
}

func TestOBDXReceive(t *testing.T) {
	bus := newBus(t)
	frames := bus.SubscribeN(context.Background(), 8)
	ctx, cancel := context.WithCancel(context.Background())
	// A junk byte the parser must resync past, then two 15-byte frames,
	// streamed in 16-byte reads: 4 reads = 2 cycles (4 frames) + 2 bytes.
	rx := append([]byte{0xFF}, bytes.Repeat(rxFrame, 2)...)
	a := &OBDXProWifi{bus: bus, conn: &fakeConn{rx: rx, reads: 4, cancel: cancel}}
	a.readLoop(ctx)
	want := gocan.NewFrame(0x7E8, []byte{0x10, 0x14, 0x5A, 0x90, 0x59, 0x53, 0x33, 0x45})
	for range 4 {
		if got := <-frames; got != want {
			t.Fatalf("delivered %s, want %s", got, want)
		}
	}
	select {
	case f := <-frames:
		t.Fatalf("partial frame delivered: %s", f)
	default:
	}

	// The loop's own setup allocates (parser, closure, read buffer); more
	// frames must not add to it.
	a = &OBDXProWifi{bus: newBus(t)} // no subscribers: Deliver itself must not allocate
	allocs := func(reads int) float64 {
		return testing.AllocsPerRun(20, func() {
			ctx, cancel := context.WithCancel(context.Background())
			a.conn = &fakeConn{rx: bytes.Repeat(rxFrame, 2), reads: reads, cancel: cancel}
			a.readLoop(ctx)
		})
	}
	if d := allocs(2000) - allocs(1000); d != 0 {
		t.Fatalf("read loop allocates %v more times per extra 1000 reads, want 0", d)
	}
}

func BenchmarkOBDXSend(b *testing.B) {
	a := &OBDXProWifi{conn: &fakeConn{}}
	f := gocan.NewFrame(0x7E0, []byte{0x02, 0x10, 0x81})
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		a.Send(ctx, f)
	}
}

// BenchmarkOBDXReceive runs one read loop over b.N reads of exactly one
// 15-byte frame each, so an op is one received frame.
func BenchmarkOBDXReceive(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &OBDXProWifi{bus: newBus(b), conn: &fakeConn{rx: rxFrame, reads: b.N, cancel: cancel}}
	b.ReportAllocs()
	b.ResetTimer()
	a.readLoop(ctx)
}
