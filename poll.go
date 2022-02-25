package gocan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan/pkg/frame"
)

type PollType int

const (
	OneOff PollType = iota
	Subscription
)

type Poll struct {
	errcount    uint16
	identifiers []uint32
	callback    chan frame.CANFrame
	//variant     PollType
}

type Hub struct {
	pollers    map[*Poll]bool
	register   chan *Poll
	unregister chan *Poll
	incoming   <-chan frame.CANFrame
}

func newPoller(size int, identifiers ...uint32) *Poll {
	return &Poll{
		identifiers: identifiers,
		callback:    make(chan frame.CANFrame, size),
		//variant:     variant,
	}
}

func waitForFrame(ctx context.Context, timeout time.Duration, p *Poll, identifiers ...uint32) (frame.CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f := <-p.callback:
		if f == nil {
			return nil, errors.New("got nil frame from poller")
		}
		return f, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", identifiers)

	}
}

func (c *Client) SendAndPoll(ctx context.Context, frame *frame.Frame, timeout time.Duration, identifiers ...uint32) (frame.CANFrame, error) {
	frame.SetTimeout(timeout)
	p := newPoller(1, identifiers...)

	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()

	if err := c.device.Send(frame); err != nil {
		return nil, err
	}
	return waitForFrame(ctx, timeout, p, identifiers...)
}

func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) chan frame.CANFrame {
	p := newPoller(100, identifiers...)
	c.hub.register <- p
	return p.callback
}

func (c *Client) Poll(ctx context.Context, timeout time.Duration, identifiers ...uint32) (frame.CANFrame, error) {
	p := newPoller(1, identifiers...)
	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()
	return waitForFrame(ctx, timeout, p, identifiers...)
}

func newHub(incoming <-chan frame.CANFrame) *Hub {
	return &Hub{
		pollers:    make(map[*Poll]bool),
		register:   make(chan *Poll, 10),
		unregister: make(chan *Poll, 10),
		incoming:   incoming,
	}
}

func (h *Hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case poll := <-h.register:
			h.pollers[poll] = true
		case poll := <-h.unregister:
			if _, ok := h.pollers[poll]; ok {
				delete(h.pollers, poll)
				close(poll.callback)
			}
		case frame := <-h.incoming:
			select {
			case poll := <-h.register:
				h.pollers[poll] = true
			default:
			}
		poll:
			for poll := range h.pollers {
				if len(poll.identifiers) == 0 {
					h.deliver(poll, frame)
					continue
				}
				for _, id := range poll.identifiers {
					if id == frame.Identifier() {
						h.deliver(poll, frame)
						continue poll
					}
				}

			}
		}
	}
}

func (h *Hub) deliver(poll *Poll, frame frame.CANFrame) {
	select {
	case poll.callback <- frame:
	default:
		poll.errcount++
	}
	if poll.errcount > 100 {
		delete(h.pollers, poll)
	}
}
