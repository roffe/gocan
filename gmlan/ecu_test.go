package gmlan

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

// fakeECU is the ECU end of the ISO-TP link, answering on 0x7E8 and modelled
// on the T8 transport. It reassembles the tester's single and multi-frame
// requests, answers a first frame with flow control (again after every block
// of consecutive frames when the block size is not 0), hands each complete
// request to handle and sends every payload handle returns as a single frame
// padded to 8 bytes. Frames a correct tester never sends are recorded as
// violations and get no answer, as the T8 ignores them. Functional requests
// on 0x101 get no answer either.
type fakeECU struct {
	handle    func(req []byte) [][]byte
	bs, stmin byte

	mu         sync.Mutex
	reqs       [][]byte
	violations []string
	msg        []byte
	want       int
	seq        byte
	block      int
	blockSize  byte
}

func (e *fakeECU) node() *gocantest.Node { return &gocantest.Node{Reply: e.reply} }

func (e *fakeECU) reply(f gocan.Frame) []gocan.Frame {
	if f.ID == 0x101 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	d := f.Bytes()
	if len(d) == 0 {
		e.violate("an empty frame")
		return nil
	}
	switch d[0] >> 4 {
	case 0:
		e.want = 0
		if n := int(d[0]); n == 0 || n > 7 || n >= len(d) {
			e.violate("a malformed single frame [% 02X]", d)
			return nil
		}
		return e.complete(d[1 : 1+d[0]])
	case 1:
		n := int(d[0]&0x0F)<<8 | int(d[1])
		if n <= 7 || len(d) != 8 {
			e.violate("a malformed first frame [% 02X]", d)
			return nil
		}
		e.msg = append(e.msg[:0], d[2:]...)
		e.want, e.seq, e.block, e.blockSize = n, 0x21, 0, e.bs
		// The T8 asks for block size 1 on apptool first frames: $1A or $3B
		// with a $1x DID.
		if (d[2] == READ_DATA_BY_IDENTIFIER || d[2] == WRITE_DATA_BY_IDENTIFIER) && d[3]&0xF0 == 0x10 {
			e.blockSize = 1
		}
		return e.flowControl()
	case 2:
		if e.want == 0 {
			e.violate("a consecutive frame [% 02X] outside a transfer", d)
			return nil
		}
		if d[0] != e.seq {
			e.violate("consecutive frame [% 02X], want sequence number %02X", d, e.seq)
			e.want = 0
			return nil
		}
		e.seq = 0x20 | (e.seq+1)&0x0F
		e.msg = append(e.msg, d[1:]...)
		if len(e.msg) >= e.want {
			req := e.msg[:e.want]
			e.want = 0
			return e.complete(req)
		}
		if len(d) != 8 {
			e.violate("a short consecutive frame [% 02X] before the end of the transfer", d)
		}
		if e.block++; e.blockSize > 0 && e.block == int(e.blockSize) {
			e.block = 0
			return e.flowControl()
		}
		return nil
	}
	e.violate("an unexpected frame [% 02X]", d)
	return nil
}

func (e *fakeECU) complete(req []byte) []gocan.Frame {
	e.reqs = append(e.reqs, slices.Clone(req))
	if e.handle == nil {
		return nil
	}
	var out []gocan.Frame
	for _, p := range e.handle(req) {
		out = append(out, sf(p...))
	}
	return out
}

func (e *fakeECU) flowControl() []gocan.Frame {
	return []gocan.Frame{gocan.NewFrame(0x7E8, []byte{0x30, e.blockSize, e.stmin, 0, 0, 0, 0, 0})}
}

func (e *fakeECU) violate(format string, args ...any) {
	e.violations = append(e.violations, fmt.Sprintf(format, args...))
}

func (e *fakeECU) requests() [][]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.reqs)
}

func (e *fakeECU) noViolations(t *testing.T) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, v := range e.violations {
		t.Errorf("tester sent %s", v)
	}
}

// check fails t on any protocol violation, and unless the ECU received
// exactly the requests want.
func (e *fakeECU) check(t *testing.T, want ...[]byte) {
	t.Helper()
	e.noViolations(t)
	if got := e.requests(); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("ECU received % 02X, want % 02X", got, want)
	}
}

func newECUClient(t *testing.T, e *fakeECU) (*Client, *gocantest.Node) {
	t.Helper()
	node := e.node()
	return newTestClient(t, node), node
}

// sf is a single frame from the ECU carrying payload, padded to 8 bytes.
func sf(payload ...byte) gocan.Frame {
	d := make([]byte, 8)
	d[0] = byte(len(payload))
	copy(d[1:], payload)
	return gocan.NewFrame(0x7E8, d)
}

// later has the ECU send payload as a single frame after d.
func later(node *gocantest.Node, d time.Duration, payload ...byte) {
	time.AfterFunc(d, func() { node.Deliver(sf(payload...)) })
}

func negative(service, code byte) []byte { return []byte{0x7F, service, code} }

func wantNRC(t *testing.T, err error, service, code byte) {
	t.Helper()
	var gm *GMError
	if !errors.As(err, &gm) {
		t.Fatalf("err = %v, want negative response %02X %02X", err, service, code)
	}
	if want := (GMError{TranslateServiceCode(service), TranslateErrorCode(code)}); *gm != want {
		t.Errorf("err = %v, want %v", gm, &want)
	}
}
