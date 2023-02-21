package gocan

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Sub struct {
	errcount    uint16
	identifiers []uint32
	callback    chan CANFrame
}

func newSub(bufferSize int, identifiers ...uint32) *Sub {
	return &Sub{
		identifiers: identifiers,
		callback:    make(chan CANFrame, bufferSize),
	}
}

func (s *Sub) Wait(ctx context.Context, timeout time.Duration) (CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f := <-s.callback:
		if f == nil {
			return nil, errors.New("got nil frame")
		}
		return f, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", s.identifiers)

	}
}

// FrameHandler takes care of faning out incoming frames to any subs
type FrameHandler struct {
	subs       map[*Sub]bool
	register   chan *Sub
	unregister chan *Sub
	incoming   <-chan CANFrame
	close      chan struct{}
}

func newFrameHandler(incoming <-chan CANFrame) *FrameHandler {
	return &FrameHandler{
		subs:       make(map[*Sub]bool),
		register:   make(chan *Sub, 10),
		unregister: make(chan *Sub, 10),
		close:      make(chan struct{}, 1),
		incoming:   incoming,
	}
}

func (h *FrameHandler) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			//			log.Println("exit framehandler")
			return
		case sub := <-h.register:
			h.sub(sub)
		case sub := <-h.unregister:
			h.unsub(sub)
		case frame := <-h.incoming:
			h.fanout(frame)
		}
	}
}

func (h *FrameHandler) Close() {
	h.close <- struct{}{}
}

func (h *FrameHandler) sub(sub *Sub) {
	h.subs[sub] = true
}

func (h *FrameHandler) fanout(frame CANFrame) {
outer:
	for sub := range h.subs {
		if len(sub.identifiers) == 0 {
			h.deliver(sub, frame)
			continue
		}
		for _, id := range sub.identifiers {
			if id == frame.Identifier() {
				h.deliver(sub, frame)
				continue outer
			}
		}
	}
}

func (h *FrameHandler) unsub(sub *Sub) {
	if _, ok := h.subs[sub]; ok {
		delete(h.subs, sub)
		close(sub.callback)
	}
}

func (h *FrameHandler) deliver(sub *Sub, frame CANFrame) {
	select {
	case sub.callback <- frame:
	default:
		sub.errcount++
	}
	if sub.errcount > 100 {
		delete(h.subs, sub)
	}
}
