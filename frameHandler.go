package gocan

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Sub struct {
	ctx         context.Context
	c           *Client
	errcount    uint32
	identifiers []uint32
	filterCount int
	callback    chan CANFrame
}

func (s *Sub) Close() {
	s.c.fh.unregister <- s
}

func (s *Sub) C() chan CANFrame {
	return s.callback
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
	closeOnce  sync.Once
}

func newFrameHandler(incoming <-chan CANFrame) *FrameHandler {
	f := &FrameHandler{
		subs:       make(map[*Sub]bool),
		register:   make(chan *Sub, 40),
		unregister: make(chan *Sub, 40),
		close:      make(chan struct{}),
		incoming:   incoming,
	}
	return f
}

func (h *FrameHandler) run(ctx context.Context) {

outer:
	for {
		select {
		case <-h.close:
			break outer
		case <-ctx.Done():
			break outer
		case sub := <-h.register:
			h.sub(sub)
		case sub := <-h.unregister:
			h.unsub(sub)
		case frame := <-h.incoming:
			h.fanout(frame)
		}
	}
	// cleanup
	for sub := range h.subs {
		delete(h.subs, sub)
		close(sub.callback)
	}
}

func (h *FrameHandler) Close() {
	h.closeOnce.Do(func() {
		close(h.close)
	})
}

func (h *FrameHandler) sub(sub *Sub) {
	h.subs[sub] = true
}

func (h *FrameHandler) fanout(frame CANFrame) {
outer:
	for sub := range h.subs {
		select {
		case <-sub.ctx.Done():
			h.unregister <- sub
			continue
		default:
			if sub.filterCount == 0 {
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
		if atomic.AddUint32(&sub.errcount, 1) > 20 {
			h.unregister <- sub
		}
	}
}
