package gocan

import (
	"context"
	"log"
	"sync"
)

// handler takes care of faning out incoming frames to any subs
type handler struct {
	adapter   Adapter
	closeCh   chan struct{}
	closeOnce sync.Once

	submap     map[uint32]map[*Subscriber]struct{}
	globalSubs []*Subscriber

	mu sync.Mutex
}

func newHandler(adapter Adapter) *handler {
	fh := &handler{
		closeCh:    make(chan struct{}),
		adapter:    adapter,
		submap:     make(map[uint32]map[*Subscriber]struct{}),
		globalSubs: make([]*Subscriber, 0, 100),
	}
	return fh
}

func (h *handler) registerSub(sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sub.filterCount == 0 {
		h.globalSubs = append(h.globalSubs, sub)
		return
	}
	for id := range sub.identifiers {
		if _, ok := h.submap[id]; !ok {
			h.submap[id] = make(map[*Subscriber]struct{})
		}
		h.submap[id][sub] = struct{}{}
	}
}

func (h *handler) unregisterSub(sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
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
				delete(subs, sub)
				if len(subs) == 0 {
					delete(h.submap, id)
				}
			}
		}
	}
	// Closing under h.mu is safe against deliver, which sends under the same
	// lock. Caller-owned channels (SubscribeChan) are never closed here.
	if sub.ownedChan {
		close(sub.responseChan)
	}
}

func (h *handler) run(ctx context.Context) {
	recvChan := h.adapter.Recv()
	for {
		select {
		case <-h.closeCh:
			return
		case <-ctx.Done():
			return
		case frame, ok := <-recvChan:
			if !ok {
				log.Println("incoming channel closed")
				return
			}
			h.deliver(frame)
		}
	}
}

func (h *handler) deliver(frame *CANFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.globalSubs {
		select {
		case sub.responseChan <- frame:
		default:
			log.Printf("%s sub full 0x%02X", sub.createdAt, frame.Identifier)
		}
	}
	if subs, ok := h.submap[frame.Identifier]; ok {
		for sub := range subs {
			select {
			case sub.responseChan <- frame:
			default:
				log.Printf("%s sub full 0x%02X", sub.createdAt, frame.Identifier)
			}
		}
	}
}

func (h *handler) close() {
	h.closeOnce.Do(func() {
		close(h.closeCh)
	})
}
