package gmlan

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

func newTestClient(t *testing.T, node *gocantest.Node) *Client {
	t.Helper()
	return NewWithOpts(gocantest.Open(t, node), WithCanID(0x7E0), WithRecvID(0x7E8), WithDefaultTimeout(50*time.Millisecond))
}

// rx is the node's answer: each payload as a frame on 0x7E8.
func rx(data ...[]byte) []gocan.Frame {
	out := make([]gocan.Frame, len(data))
	for i, d := range data {
		out[i] = gocan.NewFrame(0x7E8, d)
	}
	return out
}

// A 10-byte $1A response: first frame carries 4 bytes, one consecutive frame
// carries the remaining 6.
var (
	wantPayload = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	firstFrame  = []byte{0x10, 0x0C, 0x5A, 0x18, 1, 2, 3, 4}
	consecFrame = []byte{0x21, 5, 6, 7, 8, 9, 10, 0}
)

func isRDBI(f gocan.Frame) bool {
	b := f.Bytes()
	return len(b) >= 2 && b[1] == READ_DATA_BY_IDENTIFIER
}
func isFlowCtl(f gocan.Frame) bool { b := f.Bytes(); return len(b) >= 1 && b[0] == 0x30 }

// A multi-frame read that gets cut short used to poison every read after it:
// the consecutive frame still in flight landed in the next request's response
// window, so that request answered with the previous one's data and the client
// stayed one transaction behind forever.
func TestReadDataByIdentifierResyncsAfterAbortedTransfer(t *testing.T) {
	reads := 0
	cl := newTestClient(t, &gocantest.Node{Reply: func(req gocan.Frame) []gocan.Frame {
		switch {
		case isRDBI(req):
			reads++
			if reads > 1 {
				// The consecutive frame the first, abandoned read never
				// collected finally shows up — in this read's window.
				return rx([]byte{0x21, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA}, firstFrame)
			}
			return rx(firstFrame)
		case isFlowCtl(req) && reads > 1:
			return rx(consecFrame)
		}
		return nil // first read: node goes quiet, client times out
	}})

	if _, err := cl.ReadDataByIdentifier(context.Background(), 0x18); err == nil {
		t.Fatal("first read: want timeout, got nil error")
	}

	got, err := cl.ReadDataByIdentifier(context.Background(), 0x18)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if !bytes.Equal(got, wantPayload) {
		t.Errorf("second read = % 02X, want % 02X", got, wantPayload)
	}
}

// Traffic left over from an earlier transaction must not be mistaken for this
// request's answer, and must not eat its timeout budget either.
func TestRequestSkipsLeftoverFrames(t *testing.T) {
	cl := newTestClient(t, &gocantest.Node{Reply: func(req gocan.Frame) []gocan.Frame {
		if !isRDBI(req) {
			return nil
		}
		return rx(
			[]byte{0x24, 0xDE, 0xAD, 0xBE, 0xEF, 0, 0, 0},    // stale consecutive frame
			[]byte{0x30, 0x00, 0x00, 0, 0, 0, 0, 0},          // stale flow control
			[]byte{0x03, 0x6E, 0x00, 0x00, 0, 0, 0, 0},       // answer to another service
			[]byte{0x03, 0x7F, 0x23, 0x78, 0, 0, 0, 0},       // response pending, other service
			[]byte{0x03, 0x7F, 0x1A, 0x78, 0, 0, 0, 0},       // response pending, ours
			[]byte{0x05, 0x5A, 0x18, 0xAA, 0xBB, 0xCC, 0, 0}, // the real answer
		)
	}})

	got, err := cl.ReadDataByIdentifier(context.Background(), 0x18)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := []byte{0xAA, 0xBB, 0xCC}; !bytes.Equal(got, want) {
		t.Errorf("read = % 02X, want % 02X", got, want)
	}
}

func TestAnswers(t *testing.T) {
	rdbi := []byte{0x02, 0x1A, 0x18}
	rmba := []byte{0x06, 0x23, 0x10, 0x20, 0x30, 0x00, 0x02}
	for _, tt := range []struct {
		name string
		req  []byte
		resp []byte
		want bool
	}{
		{"positive single frame", rdbi, []byte{0x05, 0x5A, 0x18, 1, 2, 3}, true},
		{"positive first frame", rdbi, []byte{0x10, 0x0C, 0x5A, 0x18, 1, 2, 3, 4}, true},
		{"negative for our service", rdbi, []byte{0x03, 0x7F, 0x1A, 0x31}, true},
		{"T8 busy reply", rdbi, []byte{0x01, 0x60, 0, 0, 0, 0, 0, 0}, true},
		{"stale consecutive frame", rdbi, []byte{0x21, 1, 2, 3, 4, 5, 6, 7}, false},
		{"stale flow control", rdbi, []byte{0x30, 0x00, 0x00}, false},
		{"another service's answer", rdbi, []byte{0x03, 0x63, 0x00, 0x00}, false},
		{"another service's rejection", rdbi, []byte{0x03, 0x7F, 0x23, 0x31}, false},
		{"echo of our own request", rdbi, []byte{0x02, 0x1A, 0x18}, false},
		{"late answer for another DID", rdbi, []byte{0x05, 0x5A, 0x19, 1, 2, 3}, false},
		{"late first frame for another DID", rdbi, []byte{0x10, 0x0C, 0x5A, 0x19, 1, 2, 3, 4}, false},
		{"memory read answer", rmba, []byte{0x07, 0x63, 0x10, 0x20, 0x30, 1, 2}, true},
		{"memory read first frame", rmba, []byte{0x10, 0x0C, 0x63, 0x10, 0x20, 0x30, 1, 2}, true},
		{"late memory read of another address", rmba, []byte{0x07, 0x63, 0x10, 0x20, 0x40, 1, 2}, false},
		{"write answer", []byte{0x03, 0x3B, 0x18, 1}, []byte{0x02, 0x7B, 0x18}, true},
		{"late write answer for another DID", []byte{0x03, 0x3B, 0x18, 1}, []byte{0x02, 0x7B, 0x19}, false},
		// A first frame we sent is answered with flow control, not a service.
		{"flow control for our first frame", []byte{0x10, 0x0C, 0x3B, 0x18}, []byte{0x30, 0x00, 0x00}, true},
		{"stale answer during our first frame", []byte{0x10, 0x0C, 0x3B, 0x18}, []byte{0x03, 0x5A, 0x18, 0}, false},
		// A consecutive frame we sent carries no service byte to match on.
		{"final answer to our last consecutive frame", []byte{0x22, 1, 2}, []byte{0x01, 0x7B}, true},
		{"flow control after our consecutive frame", []byte{0x22, 1, 2}, []byte{0x30, 0x01, 0x00}, true},
		{"stale consecutive during our send", []byte{0x22, 1, 2}, []byte{0x24, 1, 2}, false},
	} {
		if got := answers(tt.req, tt.resp); got != tt.want {
			t.Errorf("%s: answers(% 02X, % 02X) = %v, want %v", tt.name, tt.req, tt.resp, got, tt.want)
		}
	}
}

// A bus that dies mid report is an error, not the DTCs so far.
func TestReadDTCBusDies(t *testing.T) {
	node := &gocantest.Node{}
	node.Reply = func(req gocan.Frame) []gocan.Frame {
		if b := req.Bytes(); len(b) >= 2 && b[1] == 0xA9 {
			go func() {
				node.Deliver(rx([]byte{0x03, 0x7F, 0xA9, 0x78, 0, 0, 0, 0})...)
				time.Sleep(10 * time.Millisecond)
				node.Deliver(gocan.NewFrame(0x5E8, []byte{0x81, 0x01, 0x22, 0x02, 0x12, 0, 0, 0}))
				time.Sleep(10 * time.Millisecond)
				node.Unplug(errors.New("adapter unplugged"))
			}()
		}
		return nil
	}
	cl := newTestClient(t, node)
	if dtcs, err := cl.ReadDiagnosticInformationStatusOfDTCByStatusMask(context.Background(), 0x12); err == nil {
		t.Fatalf("dead bus returned %v, nil", dtcs)
	}
}
