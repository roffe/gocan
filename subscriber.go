package gocan

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

type Subscriber struct {
	ctx          context.Context
	c            *Client
	identifiers  map[uint32]struct{}
	filterCount  int
	responseChan chan *CANFrame
	closeOnce    sync.Once
}

func (s *Subscriber) Close() {
	s.c.fh.unregister <- s
	s.closeOnce.Do(func() {
		close(s.responseChan)
	})
}

func (s *Subscriber) Chan() <-chan *CANFrame {
	return s.responseChan
}

func (s *Subscriber) Wait(ctx context.Context, timeout time.Duration) (*CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f, ok := <-s.responseChan:
		if !ok {
			return nil, ErrResponsechannelClosed
		}
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
		return nil, &TimeoutError{
			Timeout: timeout.Milliseconds(),
			Frames:  identifiers,
			Type:    "wait",
		}
	}
}

func (s *Subscriber) Deliver(f *CANFrame) {
	select {
	case s.responseChan <- f:
	default:
	}
}
