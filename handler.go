package gocan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"
)

type Sub struct {
	ctx          context.Context
	c            *Client
	identifiers  map[uint32]struct{}
	filterCount  int
	responseChan chan *CANFrame
	closeOnce    sync.Once
}

func (s *Sub) Close() {
	s.c.fh.unregister <- s
	s.closeOnce.Do(func() {
		close(s.responseChan)
	})
}

func (s *Sub) Chan() <-chan *CANFrame {
	return s.responseChan
}

func (s *Sub) Wait(ctx context.Context, timeout time.Duration) (*CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f := <-s.responseChan:
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
		return nil, fmt.Errorf("wait timeout (%dms) for frame 0x%03X", timeout.Milliseconds(), identifiers)

	}
}

// Handler takes care of faning out incoming frames to any subs
type Handler struct {
	adapter    Adapter
	subs       map[*Sub]bool
	register   chan *Sub
	unregister chan *Sub
	close      chan struct{}
	closeOnce  sync.Once
}

func newHandler(adapter Adapter) *Handler {
	f := &Handler{
		subs:       make(map[*Sub]bool),
		register:   make(chan *Sub, 40),
		unregister: make(chan *Sub, 40),
		close:      make(chan struct{}),
		adapter:    adapter,
	}
	return f
}

func (h *Handler) run(ctx context.Context) {
	defer func() {
		for sub := range h.subs {
			delete(h.subs, sub)
			//close(sub.callback)
		}
	}()
	for {
		select {
		case <-h.close:
			// log.Println("close channel closed")
			return
		case <-ctx.Done():
			// log.Println("context done")
			return
		case sub, ok := <-h.register:
			if !ok {
				log.Println("register channel closed")
				return
			}
			h.subs[sub] = true
		case sub, ok := <-h.unregister:
			if !ok {
				log.Println("unregister channel closed")
				return
			}
			delete(h.subs, sub)

		case frame, ok := <-h.adapter.Recv():
			if !ok {
				log.Println("incoming channel closed")
				return
			}
			h.processFrame(frame)
		}
	}
}

func (h *Handler) Close() {
	h.closeOnce.Do(func() {
		close(h.close)
	})
}

func (h *Handler) processFrame(frame *CANFrame) {
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
			if _, ok := sub.identifiers[frame.Identifier]; ok {
				h.deliver(sub, frame)
			}
		}
	}
}

func (h *Handler) deliver(sub *Sub, frame *CANFrame) {
	select {
	case sub.responseChan <- frame:
	default:
		//if atomic.AddUint32(&sub.errcount, 1) > 10 {
		//	h.unregister <- sub
		//}
	}
}
