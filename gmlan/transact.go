package gmlan

import (
	"context"
	"errors"
	"time"

	gocan "github.com/roffe/gocan/v2"
)

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

// response waits for the frame that answers the request, skipping leftovers
// and sitting out responsePending ($78) replies.
func (x *exchange) response(ctx context.Context) (gocan.Frame, error) {
	return x.await(ctx, func(d []byte) bool {
		return answers(x.req, d) && !isResponsePending(d)
	})
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
	if resp[0]>>4 == 1 { // first frame: [10][len][SID+0x40]...
		return len(resp) >= 3 && resp[2] == service+0x40
	}
	return resp[1] == service+0x40
}

// isResponsePending reports a negative response with code $78, which means
// "still working, keep waiting" rather than a failure.
func isResponsePending(d []byte) bool {
	return len(d) >= 4 && d[1] == 0x7F && d[3] == 0x78
}

// send transmits req on the exchange, carrying the wire hints buffered
// adapters need. expectedResponses is the number of reply frames to wait for.
func (cl *Client) send(ctx context.Context, req []byte, timeout time.Duration, expectedResponses int) error {
	if gocan.ExpectedResponses(ctx) == 0 {
		ctx = gocan.WithExpectedResponses(ctx, expectedResponses)
	}
	if gocan.ResponseTimeout(ctx) == 0 {
		ctx = gocan.WithResponseTimeout(ctx, timeout)
	}
	return cl.c.Send(ctx, gocan.NewFrame(cl.canID, req))
}
