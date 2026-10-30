package gmlan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

// GMW3110 P2CT*: after a responsePending ($78) reply the tester waits this
// long for the next answer, for at most maxResponsePending of them.
const (
	p2Star             = 5100 * time.Millisecond
	maxResponsePending = 20
)

// ISO 15765-2 allows a sender to be told to wait this many times in a row.
const maxFlowControlWaits = 10

// An exchange holds the receive subscription for one diagnostic transaction
// open from before the request is sent until the last frame of the response
// has been collected.
//
// The alternative — one subscription for the response header and a second one
// opened afterwards for the consecutive frames — desyncs permanently the first
// time a multi-frame read is cut short: the frames still in flight land in the
// *next* request's window, that request answers with the previous request's
// data, and everything after it runs one transaction behind. Holding a single
// subscription and discarding frames that cannot answer the request in hand is
// what makes a hiccup on a busy bus cost one sample instead of the session.
type exchange struct {
	ch      <-chan gocan.Frame
	cancel  context.CancelFunc
	req     []byte
	timeout time.Duration
}

// begin opens the receive window for req. It must be closed once the whole
// response has been read; close tolerates a nil exchange so callers can defer
// it before checking the error.
func (cl *Client) begin(ctx context.Context, req []byte, timeout time.Duration) *exchange {
	sctx, cancel := context.WithCancel(ctx)
	return &exchange{
		ch:      cl.c.Subscribe(sctx, cl.recvID...),
		cancel:  cancel,
		req:     req,
		timeout: timeout,
	}
}

func (x *exchange) close() {
	if x != nil {
		x.cancel()
	}
}

// await returns the next frame accepted by want and discards the rest. The
// timeout is per frame and restarts on every discarded frame — leftovers from
// an abandoned transfer must not eat the budget for the frame we are actually
// waiting for. ctx bounds the wait as a whole.
func (x *exchange) await(ctx context.Context, want func([]byte) bool) (gocan.Frame, error) {
	timer := time.NewTimer(x.timeout)
	defer timer.Stop()
	for {
		select {
		case f, alive := <-x.ch:
			if !alive {
				return gocan.Frame{}, errors.New("receive subscription closed")
			}
			if want(f.Bytes()) {
				return f, nil
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(x.timeout)
		case <-timer.C:
			return gocan.Frame{}, context.DeadlineExceeded
		case <-ctx.Done():
			return gocan.Frame{}, context.Cause(ctx)
		}
	}
}

// response waits for the frame that answers the request, skipping leftovers.
// The wait is bounded by the exchange timeout, and every responsePending
// ($78) reply to the request restarts it with P2*.
func (x *exchange) response(ctx context.Context) (gocan.Frame, error) {
	timer := time.NewTimer(x.timeout)
	defer timer.Stop()
	pending := 0
	for {
		select {
		case f, alive := <-x.ch:
			if !alive {
				return gocan.Frame{}, errors.New("receive subscription closed")
			}
			d := f.Bytes()
			if !answers(x.req, d) {
				continue
			}
			if !isResponsePending(d) {
				return f, nil
			}
			if pending++; pending > maxResponsePending {
				return gocan.Frame{}, fmt.Errorf("no final response after %d replies: %w", maxResponsePending, CheckErr(f))
			}
			timer.Reset(p2Star)
		case <-timer.C:
			return gocan.Frame{}, context.DeadlineExceeded
		case <-ctx.Done():
			return gocan.Frame{}, context.Cause(ctx)
		}
	}
}

// consecutive waits for consecutive frame seq of a multi-frame response.
// Frames left over from an abandoned transfer carry a different sequence
// number and are discarded instead of aborting this transfer; a negative
// response does abort it and is returned to the caller.
func (x *exchange) consecutive(ctx context.Context, seq byte) (gocan.Frame, error) {
	return x.await(ctx, func(d []byte) bool {
		if len(d) < 2 {
			return false
		}
		return d[0] == seq || (d[0]>>4 != 2 && len(d) >= 4 && d[1] == 0x7F)
	})
}

// answers reports whether resp can be the response to req.
//
// Whatever it rejects is a leftover from an earlier, abandoned transaction:
// consecutive and flow-control frames are never a response header, and a
// positive or negative response naming another service answers somebody
// else's request.
func answers(req, resp []byte) bool {
	if len(req) < 2 || len(resp) < 2 {
		return false
	}
	switch req[0] >> 4 {
	case 1: // we sent a first frame; the node replies with flow control
		return resp[0]>>4 == 3 || resp[1] == 0x7F
	case 2: // we sent a consecutive frame; it carries no service byte to match.
		// The node answers with flow control (block size 1) or, on the last
		// frame, the service's positive response. Only another consecutive
		// frame is certainly not ours.
		return resp[0]>>4 != 2
	}
	if resp[0]>>4 == 2 || resp[0]>>4 == 3 {
		return false
	}
	if resp[0] == 0x01 && resp[1] == 0x60 { // T8 busy reply, carries no service byte
		return true
	}
	service := req[1]
	if resp[1] == 0x7F {
		return len(resp) >= 3 && resp[2] == service
	}
	off := 1
	if resp[0]>>4 == 1 { // first frame: [10][len][SID+0x40]...
		off = 2
	}
	// A late reply for another DID or address of the same service is not ours.
	echo := req[2:min(len(req), 2+echoLen(service))]
	return len(resp) >= off+1+len(echo) && resp[off] == service+0x40 && bytes.Equal(resp[off+1:off+1+len(echo)], echo)
}

// echoLen is how many request bytes after the service byte its positive
// response repeats.
func echoLen(service byte) int {
	switch service {
	case READ_DATA_BY_IDENTIFIER, WRITE_DATA_BY_IDENTIFIER:
		return 1
	case READ_MEMORY_BY_ADDRESS:
		return 3
	}
	return 0
}

// isResponsePending reports a negative response with code $78, which means
// "still working, keep waiting" rather than a failure.
func isResponsePending(d []byte) bool {
	return len(d) >= 4 && d[1] == 0x7F && d[3] == 0x78
}

// serviceOf returns the service ID a request frame carries. A consecutive
// frame carries none.
func serviceOf(req []byte) (byte, bool) {
	switch {
	case req[0]>>4 == 0 && len(req) > 1:
		return req[1], true
	case req[0]>>4 == 1 && len(req) > 2:
		return req[2], true
	}
	return 0, false
}

// send transmits req on id, carrying the wire hints buffered adapters need.
// expectedResponses is the number of reply frames to wait for.
func (cl *Client) send(ctx context.Context, id uint32, req []byte, timeout time.Duration, expectedResponses int) error {
	if gocan.ExpectedResponses(ctx) == 0 {
		ctx = gocan.WithExpectedResponses(ctx, expectedResponses)
	}
	if gocan.ResponseTimeout(ctx) == 0 {
		ctx = gocan.WithResponseTimeout(ctx, timeout)
	}
	return cl.c.Send(ctx, gocan.NewFrame(id, req))
}

// sendWithin is send bounded by timeout.
func (cl *Client) sendWithin(ctx context.Context, req []byte, timeout time.Duration, expectedResponses int) error {
	sctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return cl.send(sctx, cl.canID, req, timeout, expectedResponses)
}

// requestService sends the service request req, in a single frame when it
// fits, and waits for the response.
func (cl *Client) requestService(ctx context.Context, req []byte, timeout time.Duration) (gocan.Frame, error) {
	if len(req) <= 7 {
		return cl.request(ctx, append([]byte{byte(len(req))}, req...), timeout)
	}
	return cl.transfer(ctx, req, timeout)
}

// transfer sends the service request req as a first frame and consecutive
// frames, paced by the node's flow control, and waits for the response. A
// busyRepeatRequest answer repeats the whole request.
func (cl *Client) transfer(ctx context.Context, req []byte, timeout time.Duration) (gocan.Frame, error) {
	if len(req) <= 6 || len(req) > 0xFF {
		return gocan.Frame{}, fmt.Errorf("a %d byte request does not fit a first frame and consecutive frames", len(req))
	}
	for attempt := 0; ; attempt++ {
		resp, err := cl.transferOnce(ctx, req, timeout)
		if err != nil || !isBusyReply(req[0], resp) || attempt >= busyRetries {
			return resp, err
		}
		if err := wait(ctx, busyRetryDelay); err != nil {
			return gocan.Frame{}, err
		}
	}
}

func (cl *Client) transferOnce(ctx context.Context, req []byte, timeout time.Duration) (gocan.Frame, error) {
	first := append([]byte{0x10, byte(len(req))}, req[:6]...)
	x := cl.begin(ctx, first, timeout)
	defer x.close()
	if err := cl.sendWithin(ctx, first, timeout, 1); err != nil {
		return gocan.Frame{}, err
	}
	seq := byte(0x21)
	for off := 6; off < len(req); {
		fc, err := x.flowControl(ctx)
		if err != nil {
			return gocan.Frame{}, err
		}
		if fc.Data[0]>>4 != 3 {
			return fc, nil
		}
		bs, stmin := int(fc.Data[1]), separationTime(fc.Data[2])
		for n := 0; off < len(req) && (bs == 0 || n < bs); n++ {
			if n > 0 && stmin > 0 {
				if err := wait(ctx, stmin); err != nil {
					return gocan.Frame{}, err
				}
			}
			end := min(off+7, len(req))
			replies := 0
			if end == len(req) || n == bs-1 {
				replies = 1
			}
			if err := cl.sendWithin(ctx, append([]byte{seq}, req[off:end]...), timeout, replies); err != nil {
				return gocan.Frame{}, err
			}
			off, seq = end, 0x20|(seq+1)&0x0F
		}
	}
	// The response answers the whole request, matched as if it had been one
	// frame.
	x.req = append([]byte{0x00}, req...)
	return x.response(ctx)
}

// flowControl waits for the node to clear the next block of consecutive
// frames, sitting out its wait frames. A negative response in its place is
// returned as is.
func (x *exchange) flowControl(ctx context.Context) (gocan.Frame, error) {
	for waits := 0; ; waits++ {
		f, err := x.await(ctx, func(d []byte) bool { return answers(x.req, d) })
		if err != nil {
			return gocan.Frame{}, fmt.Errorf("flow control: %w", err)
		}
		d := f.Bytes()
		switch {
		case d[0]>>4 != 3:
			return f, nil
		case len(d) < 3:
			return gocan.Frame{}, fmt.Errorf("short flow control [% 02X]", d)
		case d[0] == 0x30:
			return f, nil
		case d[0] == 0x31 && waits < maxFlowControlWaits:
			continue
		}
		return gocan.Frame{}, fmt.Errorf("flow control [% 02X] ended the transfer", d)
	}
}

// separationTime decodes an ISO 15765-2 STmin byte. Reserved values mean
// the longest separation defined, 127 ms.
func separationTime(b byte) time.Duration {
	switch {
	case b <= 0x7F:
		return time.Duration(b) * time.Millisecond
	case b >= 0xF1 && b <= 0xF9:
		return time.Duration(b-0xF0) * 100 * time.Microsecond
	}
	return 127 * time.Millisecond
}
