package gocan

import (
	"context"
	"log"
	"sync"
)

// handler takes care of faning out incoming frames to any subs
type handler struct {
	adapter    Adapter
	register   chan *Subscriber
	unregister chan *Subscriber
	close      chan struct{}
	closeOnce  sync.Once

	submap     map[uint32]map[*Subscriber]struct{}
	globalSubs []*Subscriber
}

func newHandler(adapter Adapter) *handler {
	f := &handler{
		//subs:       make(map[*Subscriber]bool),
		register:   make(chan *Subscriber, 40),
		unregister: make(chan *Subscriber, 40),
		close:      make(chan struct{}),
		adapter:    adapter,
		submap:     make(map[uint32]map[*Subscriber]struct{}),
	}
	return f
}

func (h *handler) run(ctx context.Context) {
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
			if sub.filterCount == 0 {
				h.globalSubs = append(h.globalSubs, sub)
				continue
			}
			for id := range sub.identifiers {
				if _, ok := h.submap[id]; !ok {
					h.submap[id] = make(map[*Subscriber]struct{})
				}
				h.submap[id][sub] = struct{}{}
			}
		case sub, ok := <-h.unregister:
			if !ok {
				log.Println("unregister channel closed")
				return
			}
			if sub.filterCount == 0 {
				for i, s := range h.globalSubs {
					if s == sub {
						h.globalSubs = append(h.globalSubs[:i], h.globalSubs[i+1:]...)
						break
					}
				}
			} else {
				for id := range sub.identifiers {
					if subs, ok := h.submap[id]; ok {
						if _, exists := subs[sub]; exists {
							delete(subs, sub)
							if len(subs) == 0 {
								delete(h.submap, id)
							}
						}
					}
				}
			}
			close(sub.responseChan)
		case frame, ok := <-recvChan:
			if !ok {
				log.Println("incoming channel closed")
				return
			}
			for _, sub := range h.globalSubs {
				select {
				case sub.responseChan <- frame:
				default:
					log.Printf("failed to deliver 0X%02X", frame.Identifier)
				}
			}
			if subs, ok := h.submap[frame.Identifier]; ok {
				for sub := range subs {
					select {
					case sub.responseChan <- frame:
					default:
						log.Printf("failed to deliver 0X%02X", frame.Identifier)
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
