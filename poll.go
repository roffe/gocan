package gocan

import (
	"context"
)

type Hub struct {
	pollers    map[*Poll]bool
	register   chan *Poll
	unregister chan *Poll
	incoming   <-chan CANFrame
}

func newHub(incoming <-chan CANFrame) *Hub {
	return &Hub{
		pollers:    make(map[*Poll]bool),
		register:   make(chan *Poll, 10),
		unregister: make(chan *Poll, 10),
		incoming:   incoming,
	}
}

type Poll struct {
	errcount    uint16
	identifiers []uint32
	callback    chan CANFrame
}

func newPoller(bufferSize int, identifiers ...uint32) *Poll {
	return &Poll{
		identifiers: identifiers,
		callback:    make(chan CANFrame, bufferSize),
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

func (h *Hub) deliver(poll *Poll, frame CANFrame) {
	select {
	case poll.callback <- frame:
	default:
		poll.errcount++
	}
	if poll.errcount > 100 {
		delete(h.pollers, poll)
	}
}
