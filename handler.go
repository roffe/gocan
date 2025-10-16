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

	//submap map[uint32]map[*Subscriber]bool
}

func newHandler(adapter Adapter) *handler {
	f := &handler{
		subs:       make(map[*Subscriber]bool),
		register:   make(chan *Subscriber, 40),
		unregister: make(chan *Subscriber, 40),
		close:      make(chan struct{}),
		adapter:    adapter,
		//submap:     make(map[uint32]map[*Subscriber]bool),
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

	recvChan := h.adapter.Recv()
	for {
		select {
		case <-h.close:
			return
		case <-ctx.Done():
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
			close(sub.responseChan)
		case frame, ok := <-recvChan:
			if !ok {
				log.Println("incoming channel closed")
				return
			}
			for sub := range h.subs {
				if sub.filterCount == 0 {
					select {
					case sub.responseChan <- frame:
					default:
						log.Println("failed to deliver 0X%02X", frame.Identifier)
					}
					continue
				}
				if _, ok := sub.identifiers[frame.Identifier]; ok {
					select {
					case sub.responseChan <- frame:
					default:
						log.Println("failed to deliver 0X%02X", frame.Identifier)
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
