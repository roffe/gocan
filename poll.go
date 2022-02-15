package gocan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan/pkg/model"
)

func (c *Client) SendAndPoll(ctx context.Context, frame *model.Frame, timeout time.Duration, identifiers ...uint32) (model.CANFrame, error) {
	p := &Poll{
		identifiers: identifiers,
		callback:    make(chan model.CANFrame),
		variant:     OneOff,
	}

	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()

	if err := c.device.Send(frame); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f := <-p.callback:
		if f == nil {
			log.Fatal("got nil frame from poller")
		}
		return f, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", identifiers)

	}
}

func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) chan model.CANFrame {
	p := &Poll{
		identifiers: identifiers,
		callback:    make(chan model.CANFrame, 10),
		variant:     Subscription,
	}
	go func() {
		<-ctx.Done()
		close(p.callback)
	}()

	c.hub.register <- p
	return p.callback
}

func (c *Client) Poll(ctx context.Context, timeout time.Duration, identifiers ...uint32) (model.CANFrame, error) {
	p := &Poll{
		identifiers: identifiers,
		callback:    make(chan model.CANFrame, 1),
		variant:     OneOff,
	}
	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()
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

type PollType int

const (
	OneOff PollType = iota
	Subscription
)

type Poll struct {
	errcount    uint16
	identifiers []uint32
	callback    chan model.CANFrame
	variant     PollType
}

type Hub struct {
	pollers    map[*Poll]bool
	register   chan *Poll
	unregister chan *Poll
	incoming   <-chan model.CANFrame
}

func newHub(incoming <-chan model.CANFrame) *Hub {
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

func (h *Hub) deliver(poll *Poll, frame model.CANFrame) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
			delete(h.pollers, poll)
		}
	}()
	select {
	case poll.callback <- frame:
	default:
		poll.errcount++
	}
	if poll.errcount > 100 { // after 100 failed delieveries you are gone
		//log.Println("major slacker this one")
		delete(h.pollers, poll)
	}
}
