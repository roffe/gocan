package canusb

import (
	"fmt"
	"log"
	"time"
)

func (c *Canusb) Poll(identifier uint16, timeout time.Duration) (*Frame, error) {
	p := &Poll{
		identifier: identifier,
		callback:   make(chan *Frame),
	}
	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()
	select {
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
	identifier uint16
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
		incoming:   make(chan *Frame, 10),
	}
}

func (h *Hub) run() {
	for {
		select {
		case poll := <-h.register:
			h.pollers[poll] = true
		case poll := <-h.unregister:
			if _, ok := h.pollers[poll]; ok {
				delete(h.pollers, poll)
				close(poll.callback)
			}
		case frame := <-h.incoming:
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
