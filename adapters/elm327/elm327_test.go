package elm327

import (
	"bytes"
	"context"
	"testing"

	gocan "github.com/roffe/gocan/v2"
	"go.bug.st/serial"
)

var replyOK = []byte(respOK + ">")

// elmPort answers AT commands with OK and frame payloads with frameReply,
// allocation-free. The embedded nil serial.Port panics on anything else.
type elmPort struct {
	serial.Port
	frameReply []byte
	reply      []byte
	pos        int
	writes     []string // recorded while record is set
	record     bool
}

func (p *elmPort) Write(b []byte) (int, error) {
	if p.record {
		p.writes = append(p.writes, string(b))
	}
	p.reply, p.pos = p.frameReply, 0
	if bytes.HasPrefix(b, []byte("AT")) {
		p.reply = replyOK
	}
	return len(b), nil
}

func (p *elmPort) Read(b []byte) (int, error) {
	n := copy(b, p.reply[p.pos:])
	p.pos += n
	return n, nil
}

func newELM(tb testing.TB, fp *elmPort) (*ELM327, *gocan.Bus) {
	tb.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), &gocan.Loopback{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { bus.Close() })
	return &ELM327{bus: bus, port: fp}, bus
}

func TestELM327Send(t *testing.T) {
	fp := &elmPort{frameReply: []byte("7E8064100BE3FA813\r7E810\r\r>"), record: true}
	el, bus := newELM(t, fp)
	frames := bus.SubscribeN(context.Background(), 8)
	resp := gocan.WithExpectedResponses(context.Background(), 2)

	if err := el.Send(resp, gocan.NewFrame(0x7E0, []byte{0x02, 0x01, 0x00})); err != nil {
		t.Fatal(err)
	}
	fp.frameReply = []byte("\r>")
	if err := el.Send(context.Background(), gocan.NewFrame(0x7E0, nil)); err != nil {
		t.Fatal(err)
	}
	if err := el.Send(context.Background(), gocan.NewFrame(0x266, []byte{0x40, 0xA1, 0x3F, 0x80, 0xAB})); err != nil {
		t.Fatal(err)
	}
	for _, f := range []gocan.Frame{ // ATSH is %03X: padded to 3 digits, never truncated
		gocan.NewFrame(0x7, []byte{0x01}),
		gocan.NewFrame(0x1ABC, []byte{0xFF}),
		gocan.NewExtendedFrame(0x18DAF110, []byte{0x3E}),
	} {
		if err := el.Send(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"ATSH7E0\r", "ATR1\r", "020100\r", "ATR0\r", "00\r", "ATSH266\r", "40A13F80AB\r",
		"ATSH007\r", "01\r", "ATSH1ABC\r", "FF\r", "ATSH18DAF110\r", "3E\r"}
	if len(fp.writes) != len(want) {
		t.Fatalf("writes %q, want %q", fp.writes, want)
	}
	for i := range want {
		if fp.writes[i] != want[i] {
			t.Fatalf("write %d: %q, want %q", i, fp.writes[i], want[i])
		}
	}
	for _, w := range []gocan.Frame{
		gocan.NewFrame(0x7E8, []byte{0x06, 0x41, 0x00, 0xBE, 0x3F, 0xA8, 0x13}),
		gocan.NewFrame(0x7E8, []byte{0x10}),
	} {
		if got := <-frames; got != w {
			t.Fatalf("delivered %s, want %s", got, w)
		}
	}
}

// Send is the per-frame hot path, including the ATSH/ATR toggles a T7
// session does on every frame (requests on 0x240 expect a reply, acks on
// 0x266 do not).
func TestELM327SendNoAlloc(t *testing.T) {
	fp := &elmPort{frameReply: []byte("7E8064100BE3FA813\r7E810\r\r>")}
	el, _ := newELM(t, fp) // no subscribers: Deliver itself must not allocate
	resp := gocan.WithExpectedResponses(context.Background(), 2)
	req := gocan.NewFrame(0x240, []byte{0x40, 0xA1, 0x02, 0x21, 0x90})
	ack := gocan.NewFrame(0x266, []byte{0x40, 0xA1, 0x3F, 0x80})
	if n := testing.AllocsPerRun(100, func() {
		el.Send(resp, req)
		el.Send(context.Background(), ack)
	}); n != 0 {
		t.Fatalf("Send allocates %v times per request/ack pair, want 0", n)
	}
}

func BenchmarkELM327Send(b *testing.B) {
	resp := gocan.WithExpectedResponses(context.Background(), 2)
	b.Run("steady", func(b *testing.B) {
		el, _ := newELM(b, &elmPort{frameReply: []byte("7E8064100BE3FA813\r7E810\r\r>")})
		f := gocan.NewFrame(0x7E0, []byte{0x02, 0x01, 0x00})
		b.ReportAllocs()
		for b.Loop() {
			el.Send(resp, f)
		}
	})
	b.Run("t7pair", func(b *testing.B) {
		el, _ := newELM(b, &elmPort{frameReply: []byte("25840A1DEADBEEF0102\r\r>")})
		req := gocan.NewFrame(0x240, []byte{0x40, 0xA1, 0x02, 0x21, 0x90})
		ack := gocan.NewFrame(0x266, []byte{0x40, 0xA1, 0x3F, 0x80})
		b.ReportAllocs()
		for b.Loop() {
			el.Send(resp, req)
			el.Send(context.Background(), ack)
		}
	})
}
