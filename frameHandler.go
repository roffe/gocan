package gocan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type Sub struct {
	ctx         context.Context
	c           *Client
	errcount    uint32
	identifiers map[uint32]struct{}
	filterCount int
	callback    chan CANFrame
}

func (s *Sub) Close() {
	s.c.fh.unregister <- s
}

func (s *Sub) Chan() <-chan CANFrame {
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
		identifiers := make([]uint32, 0, len(s.identifiers))
		for id := range s.identifiers {
			identifiers = append(identifiers, id)
		}
		slices.Sort(identifiers)
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", identifiers)

	}
}

// FrameHandler takes care of faning out incoming frames to any subs
type FrameHandler struct {
	adapter    Adapter
	subs       map[*Sub]bool
	register   chan *Sub
	unregister chan *Sub
	close      chan struct{}
	closeOnce  sync.Once
}

func newFrameHandler(adapter Adapter) *FrameHandler {
	f := &FrameHandler{
		subs:       make(map[*Sub]bool),
		register:   make(chan *Sub, 40),
		unregister: make(chan *Sub, 40),
		close:      make(chan struct{}),
		adapter:    adapter,
	}
	return f
}

func (h *FrameHandler) run(ctx context.Context) {
	defer func() {
		for sub := range h.subs {
			delete(h.subs, sub)
			close(sub.callback)
		}
	}()
	for {
		select {
		case <-h.close:
			//log.Println("close channel closed")
			return
		case <-ctx.Done():
			//log.Println("context done")
			return
		case sub, ok := <-h.register:
			if !ok {
				log.Println("register channel closed")
				return
			}
			h.sub(sub)
		case sub, ok := <-h.unregister:
			if !ok {
				log.Println("unregister channel closed")
				return
			}
			h.unsub(sub)
		case frame, ok := <-h.adapter.Recv():
			if !ok {
				log.Println("incoming channel closed")
				return
			}
			h.processFrame(frame)
		}
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

func (h *FrameHandler) processFrame(frame CANFrame) {
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
			if _, ok := sub.identifiers[frame.Identifier()]; ok {
				h.deliver(sub, frame)
			}
		}
	}
}

func (h *FrameHandler) unsub(sub *Sub) {
	if _, ok := h.subs[sub]; ok {
		close(sub.callback)
		delete(h.subs, sub)
	}
}

func (h *FrameHandler) deliver(sub *Sub, frame CANFrame) {
	select {
	case sub.callback <- frame:
	default:
		if atomic.AddUint32(&sub.errcount, 1) > 10 {
			h.unregister <- sub
		}
	}
}
