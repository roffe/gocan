package gocan

import (
	"context"
	"log"
	"sync"
)

// handler takes care of faning out incoming frames to any subs
type handler struct {
	adapter    Adapter
	subs       map[*Subscriber]bool
	register   chan *Subscriber
	unregister chan *Subscriber
	close      chan struct{}
	closeOnce  sync.Once
}

func newHandler(adapter Adapter) *handler {
	f := &handler{
		subs:       make(map[*Subscriber]bool),
		register:   make(chan *Subscriber, 40),
		unregister: make(chan *Subscriber, 40),
		close:      make(chan struct{}),
		adapter:    adapter,
	}
	return f
}

func (h *handler) run(ctx context.Context) {
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
			for sub := range h.subs {
				select {
				case <-sub.ctx.Done():
					h.unregister <- sub
					continue
				default:
					if sub.filterCount == 0 {
						sub.Deliver(frame)
						continue
					}
					if _, ok := sub.identifiers[frame.Identifier]; ok {
						sub.Deliver(frame)
					}
				}
			}
		}
	}
}

func (h *handler) Close() {
	h.closeOnce.Do(func() {
		close(h.close)
	})
}
