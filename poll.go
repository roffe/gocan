package canusb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

func (c *Canusb) SendAndPoll(ctx context.Context, frame *Frame, pollIdentifier uint32, timeout time.Duration) (*Frame, error) {
	p := &Poll{
		identifier: pollIdentifier,
		callback:   make(chan *Frame),
	}

	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()

	select {
	case c.send <- frame:
	default:
		return nil, errors.New("failed to put frame on queue")
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
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", pollIdentifier)

	}
}

func (c *Canusb) Poll(ctx context.Context, identifier uint32, timeout time.Duration) (*Frame, error) {
	p := &Poll{
		identifier: identifier,
		callback:   make(chan *Frame),
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
			log.Fatal("got nil frame from poller")
		}
		return f, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", identifier)
	}
}

type Poll struct {
	errcount   uint16
	identifier uint32
	callback   chan *Frame
}

func (p *Poll) String() string {
	return fmt.Sprintf("0x%03x %d", p.identifier, p)
}

type Hub struct {
	pollers    map[*Poll]bool
	register   chan *Poll
	unregister chan *Poll
	incoming   chan *Frame
}

func newHub() *Hub {
	return &Hub{
		pollers:    make(map[*Poll]bool),
		register:   make(chan *Poll, 10),
		unregister: make(chan *Poll, 10),
		incoming:   make(chan *Frame, 16),
	}
}

func (h *Hub) run(ctx context.Context, wg *sync.WaitGroup) {
	wg.Done()
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

			for poll := range h.pollers {
				if poll.identifier == frame.Identifier || poll.identifier == 0 {
					select {
					case poll.callback <- frame:
					default:
						poll.errcount++
					}
					if poll.errcount > 100 { // after 100 failed publishes you are gone
						//log.Println("major slacker this one")
						delete(h.pollers, poll)
					}
				}
			}
		}
	}
}
