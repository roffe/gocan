package gocan

import (
	"context"
)

type Mock struct {
	BaseAdapter
}

// Create a new Mock adapter used for testing
func NewMock(name string, cfg *AdapterConfig) (Adapter, error) {
	return &Template{
		BaseAdapter: NewBaseAdapter(name, cfg),
	}, nil
}

func (v *Mock) Open(ctx context.Context) error {
	go v.sendManager(ctx)
	go v.recvManager(ctx)
	return nil
}

func (v *Mock) Close() error {
	v.BaseAdapter.Close()
	return nil
}

func (v *Mock) recvManager(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-v.closeChan:
			return
		}
	}
}

func (v *Mock) sendManager(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-v.closeChan:
			return
		case frame := <-v.sendChan:
			frame.FrameType = Incoming
			select {
			case v.recvChan <- frame:
			default:
				v.sendErrorEvent(ErrDroppedFrame)
			}
		}
	}
}
