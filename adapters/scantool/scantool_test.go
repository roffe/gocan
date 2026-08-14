package scantool

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

// stnPort emulates an STN11xx UART per the FRPM: command/response up to a
// '>' prompt, echo on at power-up, and the documented STBR handshake (OK at
// the old baud, STI banner at the new baud, CR to confirm). Output emitted
// at a baud other than the host's reads as garbage; host writes at the
// wrong baud dirty the device line buffer so the next command answers '?'.
type stnPort struct {
	mu       sync.Mutex
	hostBaud int
	devBaud  int
	echo     bool
	awaitCR  bool    // banner printed, waiting for the confirming CR
	dirty    bool    // device line buffer holds garbage
	out      []chunk // pending device output
	wrote    []string
	closed   bool
}

type chunk struct {
	baud int
	data []byte
}

func (p *stnPort) emit(s string) {
	p.out = append(p.out, chunk{baud: p.devBaud, data: []byte(s)})
}

func (p *stnPort) command(cmd string) {
	if p.echo {
		p.emit(cmd + "\r")
	}
	if p.dirty {
		p.dirty = false
		p.emit("?\r>")
		return
	}
	switch {
	case cmd == "":
		if p.awaitCR {
			p.awaitCR = false
			p.emit("OK\r>")
			return
		}
		p.emit(">") // repeat-last-command elided
	case cmd == "ATE0":
		p.echo = false
		p.emit("OK\r>")
	case cmd == "STI":
		p.emit("STN1170 v4.2.0\r>")
	case strings.HasPrefix(cmd, "STBRT"):
		p.emit("OK\r>")
	case strings.HasPrefix(cmd, "STBR"):
		baud, err := strconv.Atoi(cmd[4:])
		if err != nil || baud > 2_000_000 {
			p.emit("?\r>")
			return
		}
		p.emit("OK\r") // at the old baud, no prompt follows
		p.devBaud = baud
		p.emit("STN1170 v4.2.0\r") // at the new baud
		p.awaitCR = true
	default:
		p.emit("OK\r>")
	}
}

func (p *stnPort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	p.wrote = append(p.wrote, string(b))
	if p.hostBaud != p.devBaud {
		p.dirty = true // arrives as line noise
		return len(b), nil
	}
	p.command(strings.TrimSuffix(string(b), "\r"))
	return len(b), nil
}

func (p *stnPort) Read(b []byte) (int, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	if len(p.out) == 0 {
		p.mu.Unlock()
		time.Sleep(time.Millisecond) // read timeout, nothing arrived
		return 0, nil
	}
	c := p.out[0]
	n := copy(b, c.data)
	if n == len(c.data) {
		p.out = p.out[1:]
	} else {
		p.out[0].data = c.data[n:]
	}
	if c.baud != p.hostBaud {
		for i := range n {
			b[i] = 0xAA // wrong baud: garbage
		}
	}
	p.mu.Unlock()
	return n, nil
}

func (p *stnPort) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return nil
}

func (p *stnPort) SetBaud(baud int) error {
	p.mu.Lock()
	p.hostBaud = baud
	p.mu.Unlock()
	return nil
}

func (p *stnPort) SetReadTimeout(time.Duration) error { return nil }

func (p *stnPort) ResetInputBuffer() error {
	p.mu.Lock()
	p.out = nil
	p.mu.Unlock()
	return nil
}

func (p *stnPort) ResetOutputBuffer() error { return nil }

func (p *stnPort) writes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.wrote...)
}

func openScantool(t *testing.T, fp *stnPort) *gocan.Bus {
	t.Helper()
	a, err := New(OBDLinkSX, gocan.Config{CANRate: 500})
	if err != nil {
		t.Fatal(err)
	}
	a.(*Scantool).port = fp
	bus, err := gocan.OpenAdapter(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	return bus
}

// A device fresh out of power-up (115.2 kbps, echo on) must be walked
// through the full STBR handshake and end up configured at 2 Mbit.
func TestOpenSwitchesBaud(t *testing.T) {
	fp := &stnPort{devBaud: 115200, echo: true}
	bus := openScantool(t, fp)

	if fp.devBaud != 2_000_000 {
		t.Fatalf("device baud after open: %d", fp.devBaud)
	}
	got := fp.writes()
	idx := func(w string) int {
		for i, g := range got {
			if g == w {
				return i
			}
		}
		t.Fatalf("write %q missing from %q", w, got)
		return -1
	}
	if !(idx("ATE0\r") < idx("STBRT250\r") && idx("STBRT250\r") < idx("STBR2000000\r") && idx("STBR2000000\r") < idx("STCMM1\r") && idx("STCMM1\r") < idx("ATCF000\r")) {
		t.Fatalf("handshake out of order: %q", got)
	}

	bus.Close()
	last := fp.writes()
	if last[len(last)-1] != "ATZ\r" {
		t.Fatalf("close should reset, got %q", last[len(last)-1])
	}
}

// A device left at 2 Mbit by a crashed session (echo already off, line
// buffer dirtied by probes at the wrong baud) must be found via the
// fast path without any STBR exchange.
func TestOpenFastPathAtTarget(t *testing.T) {
	fp := &stnPort{devBaud: 2_000_000}
	openScantool(t, fp)

	for _, w := range fp.writes() {
		if strings.HasPrefix(w, "STBR") {
			t.Fatalf("fast path issued %q", w)
		}
	}
	if fp.devBaud != 2_000_000 {
		t.Fatalf("device baud after open: %d", fp.devBaud)
	}
}
