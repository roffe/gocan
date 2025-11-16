package gocan

import (
	"context"
	"fmt"
	"sync"
)

type Subscriber struct {
	createdAt    string
	cl           *Client
	identifiers  map[uint32]struct{}
	filterCount  int
	responseChan chan *CANFrame
	closeOnce    sync.Once
}

func (s *Subscriber) Close() {
	s.closeOnce.Do(func() {
		s.cl.fh.unregisterSub(s)
	})
}

func (s *Subscriber) Chan() <-chan *CANFrame {
	return s.responseChan
}

func (s *Subscriber) wait(ctx context.Context) (*CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout: %w", ctx.Err())
	case frame, ok := <-s.responseChan:
		if !ok {
			return nil, ErrResponsechannelClosed
		}
		return frame, nil
	}
}
