package t7kwp

import (
	"bytes"
	"context"
	"testing"

	"github.com/roffe/gocan/v2"
)

// fastECU models the firmware side of the fast transfer (YDiaga.c, X_can.c,
// XEolprg.c): KWP reassembly on 0x240, responses on 0x258 in ack mode, the
// upload stream on 0x7C1 and armed download blocks on 0x7C0.
type fastECU struct {
	bus   *gocan.Bus
	stock bool   // answer the probe like SAAB firmware
	mem   []byte // what uploads read
	flash []byte // what commits write

	dropUp   int // drop this upload data frame once (1-based, 0 = never)
	dropDown int // drop this download data frame once

	msg     []byte
	pending []byte // second response frame, sent on the tester's 0x266 ack

	armed       bool
	adr, n, cnt int
	buf         []byte
	s1, s2      uint32
	upFrames    int
	downFrames  int
}

func (e *fastECU) Open(_ context.Context, b *gocan.Bus) error { e.bus = b; return nil }
func (e *fastECU) Close() error                               { return nil }

func (e *fastECU) Send(_ context.Context, f gocan.Frame) error {
	switch f.ID {
	case 0x240:
		if f.Data[0]&0x40 != 0 {
			e.msg = append([]byte(nil), f.Data[2:f.Length]...)
		} else {
			e.msg = append(e.msg, f.Data[2:f.Length]...)
		}
		if f.Data[0]&0x3F == 0 {
			e.handle(e.msg[:1+int(e.msg[0])])
		}
	case 0x266:
		if e.pending != nil {
			p := e.pending
			e.pending = nil
			e.bus.Deliver(gocan.NewFrame(0x258, p))
		}
	case FAST_DOWN_ID:
		e.downFrames++
		if e.downFrames == e.dropDown {
			return nil
		}
		if !e.armed {
			return nil
		}
		for _, c := range f.Data {
			if e.cnt < e.n {
				e.buf[e.cnt] = c
				e.cnt++
				e.s1 += uint32(c)
				e.s2 += e.s1
			}
		}
	}
	return nil
}

// respond sends a KWP response: one frame, or two with the ack handshake.
func (e *fastECU) respond(p ...byte) {
	kwp := append([]byte{byte(len(p))}, p...)
	if len(kwp) <= 6 {
		e.bus.Deliver(gocan.NewFrame(0x258, append([]byte{0xC0, 0xA1}, kwp...)))
		return
	}
	e.pending = append([]byte{0x80, 0xA1}, kwp[6:]...)
	e.bus.Deliver(gocan.NewFrame(0x258, append([]byte{0xC1, 0xA1}, kwp[:6]...)))
}

func (e *fastECU) handle(m []byte) {
	if m[1] != START_ROUTINE_BY_IDENTIFIER {
		e.respond(0x7F, m[1], 0x11)
		return
	}
	u24 := func(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
	switch {
	case m[2] == RLI_FAST_INFO && !e.stock:
		e.respond(0x71, RLI_FAST_INFO, 1, 0x20, 0x00)
	case m[2] == RLI_FAST_UPLOAD && !e.stock:
		adr, n := u24(m[3:]), u24(m[6:])
		var s1, s2 uint32
		for i := 0; i < n; i += 8 {
			var buf [8]byte
			for j := range 8 {
				if i+j < n {
					buf[j] = e.mem[adr+i+j]
					s1 += uint32(buf[j])
					s2 += s1
				}
			}
			e.upFrames++
			if e.upFrames != e.dropUp {
				e.bus.Deliver(gocan.NewFrame(FAST_UP_ID, buf[:]))
			}
		}
		e.respond(0x71, RLI_FAST_UPLOAD, byte(s2>>24), byte(s2>>16), byte(s2>>8), byte(s2))
	case m[2] == RLI_FAST_DOWNLOAD && !e.stock:
		e.adr, e.n = u24(m[3:]), u24(m[6:])
		if (e.adr|e.n)&1 != 0 || e.n == 0 || e.n > 0x2000 {
			e.respond(0x7F, 0x31, 0x42)
			return
		}
		e.buf, e.cnt, e.s1, e.s2, e.armed = make([]byte, e.n), 0, 0, 0, true
		e.respond(0x71, RLI_FAST_DOWNLOAD)
	case m[2] == RLI_FAST_COMMIT && !e.stock:
		sum := uint32(m[3])<<24 | uint32(m[4])<<16 | uint32(m[5])<<8 | uint32(m[6])
		switch {
		case !e.armed:
			e.respond(0x7F, 0x31, 0x22)
		case e.cnt != e.n:
			e.armed = false
			e.respond(0x7F, 0x31, 0x79)
		case e.s2 != sum:
			e.armed = false
			e.respond(0x7F, 0x31, 0x77)
		default:
			e.armed = false
			copy(e.flash[e.adr:], e.buf)
			e.respond(0x71, RLI_FAST_COMMIT)
		}
	default:
		e.respond(0x7F, 0x31, 0x12)
	}
}

func newFastECU(t *testing.T, e *fastECU) *Client {
	t.Helper()
	bus, err := gocan.OpenAdapter(t.Context(), e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	c := New(bus)
	c.SetResponseID(0x258)
	return c
}

func testImage(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*131 + i>>8)
	}
	return b
}

func TestFastSum(t *testing.T) {
	// s1 = 1, 3, 6; s2 = 1 + 3 + 6
	if got := FastSum([]byte{1, 2, 3}); got != 10 {
		t.Fatalf("FastSum = %d, want 10", got)
	}
	// position-weighted: a dropped frame plus a duplicated one keeps a plain
	// byte sum but not this one
	a := []byte{9, 0, 0, 0, 7, 0}
	b := []byte{9, 0, 0, 0, 0, 7}
	if FastSum(a) == FastSum(b) {
		t.Fatal("FastSum does not see a shifted byte")
	}
}

func TestFastInfo(t *testing.T) {
	block, ok, err := newFastECU(t, &fastECU{stock: true}).FastInfo(t.Context())
	if err != nil || ok {
		t.Fatalf("stock ECU: ok=%v err=%v, want not available", ok, err)
	}
	block, ok, err = newFastECU(t, &fastECU{}).FastInfo(t.Context())
	if err != nil || !ok || block != 0x2000 {
		t.Fatalf("fast ECU: block=%#x ok=%v err=%v", block, ok, err)
	}
}

func TestFastUpload(t *testing.T) {
	mem := testImage(0x10000)
	for _, n := range []uint32{1, 8, 9, 0x1235} {
		got, err := newFastECU(t, &fastECU{mem: mem}).FastUpload(t.Context(), 0x100, n)
		if err != nil {
			t.Fatalf("len %#x: %v", n, err)
		}
		if !bytes.Equal(got, mem[0x100:0x100+n]) {
			t.Fatalf("len %#x: data mismatch", n)
		}
	}

	c := newFastECU(t, &fastECU{mem: mem, dropUp: 17})
	if _, err := c.FastUpload(t.Context(), 0, 0x1000); err == nil {
		t.Fatal("dropped frame not detected")
	}
	if got, err := c.FastUpload(t.Context(), 0, 0x1000); err != nil || !bytes.Equal(got, mem[:0x1000]) {
		t.Fatalf("retry after drop: %v", err)
	}
}

func TestFastDownloadBlock(t *testing.T) {
	img := testImage(0x2000)
	e := &fastECU{flash: make([]byte, 0x10000), dropDown: 40}
	c := newFastECU(t, e)

	if err := c.FastDownloadBlock(t.Context(), 0x4000, img); err == nil {
		t.Fatal("dropped frame not detected")
	}
	if bytes.Contains(e.flash, img[:64]) {
		t.Fatal("unverified block was programmed")
	}
	if err := c.FastDownloadBlock(t.Context(), 0x4000, img); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(e.flash[0x4000:0x6000], img) {
		t.Fatal("flash mismatch")
	}
	if err := c.FastDownloadBlock(t.Context(), 0x4001, img[:2]); err == nil {
		t.Fatal("odd address accepted")
	}
}
