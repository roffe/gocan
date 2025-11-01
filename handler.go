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

	onEvent func(Event)

	mu sync.RWMutex
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

func (h *handler) registerSubscriber(sub *Subscriber) {
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

func (h *handler) unregisterSubscriber(sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sub.filterCount == 0 {
		for i, s := range h.globalSubs {
			if s == sub {
				h.globalSubs = append(h.globalSubs[:i], h.globalSubs[i+1:]...)
				break
			}
		}
		close(sub.responseChan)
		return
	}
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
	close(sub.responseChan)
}

func (h *handler) run(ctx context.Context) {
	recvChan := h.adapter.Recv()
	eventChan := h.adapter.Event()
	for {
		select {
		case <-h.closeCh:
			return
		case <-ctx.Done():
			return
		case evt, ok := <-eventChan:
			if !ok {
				log.Println("event channel closed")
				return
			}
			if h.onEvent != nil {
				go h.onEvent(evt)
			}
		case frame, ok := <-recvChan:
			if !ok {
				log.Println("incoming channel closed")
				return
			}
			h.deliver(frame)
		}
	}
}

// NOTE: We send while holding RLock on h.mu. unregisterSubscriber acquires the write lock
// and closes sub.responseChan. Holding RLock guarantees the channel won't be closed
// mid-send, avoiding send-on-closed-channel panics.
func (h *handler) deliver(frame *CANFrame) {
	h.mu.RLock()
	defer h.mu.RUnlock()
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

func (h *handler) close() {
	h.closeOnce.Do(func() {
		close(h.closeCh)
	})
}

// Set a function to be called on events
func (h *handler) setOnEvent(fn func(Event)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onEvent = fn
}
