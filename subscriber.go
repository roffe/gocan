package gocan

import (
	"context"
	"fmt"
	"sync"
)

// Subscriber receives CAN frames matching a set of identifiers. Create one
// with Client.Subscribe, Client.SubscribeFunc or Client.SubscribeChan and
// release it with Close.
type Subscriber struct {
	createdAt   string
	cl          *Client
	identifiers map[uint32]struct{}
	filterCount int
	// responseChan carries matched frames. It is closed on Close only if
	// ownedChan is set; channels supplied by the caller via SubscribeChan
	// are never closed by the library.
	responseChan chan *CANFrame
	ownedChan    bool
	closeOnce    sync.Once
}

// Close unregisters the subscriber. If the delivery channel was created by
// the library it is closed, ending any range over Chan. Close is safe to
// call multiple times and is also invoked automatically when the context
// given at subscription time is cancelled.
func (s *Subscriber) Close() {
	s.closeOnce.Do(func() {
		s.cl.fh.unregisterSub(s)
	})
}

// Chan returns the channel frames are delivered on.
func (s *Subscriber) Chan() <-chan *CANFrame {
	return s.responseChan
}

func (s *Subscriber) wait(ctx context.Context) (*CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout: %w", ctx.Err())
	case <-s.cl.ctx.Done():
		if err := s.cl.Err(); err != nil {
			return nil, err
		}
		return nil, ErrClosed
	case frame, ok := <-s.responseChan:
		if !ok {
			return nil, ErrResponseChannelClosed
		}
		return frame, nil
	}
}
