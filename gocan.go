//go:generate protoc.exe --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative .\proto\server.proto

package gocan

import (
	"context"
	"errors"
	"sync"
	"time"
)

// We have 3 bits allowing 8 different system messages hidden in a 29bit can id stored in a uint32
const (
	SystemMsg uint32 = 0x80000000 + iota
	SystemMsgError
	SystemMsgUnrecoverableError
	SystemMsgWBLReading
	SystemMsgDataResponse
	SystemMsgDataRequest
	SystemMsgWriteResponse
	SystemMsgUnknown
)

type Opts func(*Client)

type Client struct {
	fh        *handler
	adapter   Adapter
	closeOnce sync.Once
}

func New(ctx context.Context, adapterName string, cfg *AdapterConfig) (*Client, error) {
	adapter, err := NewAdapter(adapterName, cfg)
	if err != nil {
		return nil, err
	}
	return NewWithAdapter(ctx, adapter)
}

func NewWithAdapter(ctx context.Context, adapter Adapter) (*Client, error) {
	return NewWithOpts(ctx, adapter)
}

func NewWithOpts(ctx context.Context, adapter Adapter, opts ...Opts) (*Client, error) {
	c := &Client{
		fh:      newHandler(adapter),
		adapter: adapter,
	}

	for _, opt := range opts {
		opt(c)
	}

	go c.fh.run(ctx)

	if err := adapter.Open(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) Adapter() Adapter {
	return c.adapter
}

func (c *Client) Err() <-chan error {
	return c.adapter.Err()
}

//func (c *Client) SetFilter(filters []uint32) error {
//	return c.adapter.SetFilter(filters)
//}

func (c *Client) Close() (err error) {
	c.closeOnce.Do(func() {
		err = c.adapter.Close()
		c.fh.Close()
	})
	return err
}

// Send a CAN Frame
func (c *Client) SendFrame(msg *CANFrame) error {
	select {
	case c.adapter.Send() <- msg:
		return nil
	case <-time.After(5 * time.Second):
		// default:
		return errors.New("timeout sending frame (5s)")
	}
}

// Shortcommand to send a standard 11bit frame
func (c *Client) Send(identifier uint32, data []byte, f CANFrameType) error {
	return c.SendFrame(NewFrame(identifier, data, f))
}

// Shortcommand to send a extended 29bit frame
func (c *Client) SendExtended(identifier uint32, data []byte, f CANFrameType) error {
	return c.SendFrame(NewExtendedFrame(identifier, data, f))
}

// Send and wait up to <timeout> for a answer on given identifiers
func (c *Client) SendAndWait(ctx context.Context, frame *CANFrame, timeout time.Duration, identifiers ...uint32) (*CANFrame, error) {
	frame.Timeout = timeout
	sub := newSub(ctx, c, 1, identifiers...)
	select {
	case c.fh.register <- sub:
	default:
		return nil, ErrFramhandlerRegisterSub
	}
	defer func() {
		c.fh.unregister <- sub
	}()
	if err := c.SendFrame(frame); err != nil {
		return nil, err
	}
	return sub.Wait(ctx, timeout)
}

// Wait for a certain CAN identifier for up to <timeout>
func (c *Client) Wait(ctx context.Context, timeout time.Duration, identifiers ...uint32) (*CANFrame, error) {
	sub := newSub(ctx, c, 1, identifiers...)
	select {
	case c.fh.register <- sub:
	default:
		return nil, ErrFramhandlerRegisterSub
	}
	defer func() {
		c.fh.unregister <- sub
	}()
	return sub.Wait(ctx, timeout)
}

func (c *Client) SubscribeFunc(ctx context.Context, f func(*CANFrame), identifiers ...uint32) *Subscriber {
	sub := newSub(ctx, c, 20, identifiers...)
	c.fh.register <- sub
	go func() {
		defer func() {
			c.fh.unregister <- sub
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-sub.responseChan:
				if !ok {
					return
				}
				f(frame)
			}
		}
	}()
	return sub
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) SubscribeChan(ctx context.Context, channel chan *CANFrame, identifiers ...uint32) *Subscriber {
	sub := newSub(ctx, c, 20, identifiers...)
	sub.responseChan = channel
	c.fh.register <- sub
	return sub
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) *Subscriber {
	sub := newSub(ctx, c, 20, identifiers...)
	c.fh.register <- sub
	return sub
}

func newSub(ctx context.Context, c *Client, bufferSize int, identifiers ...uint32) *Subscriber {
	return &Subscriber{
		ctx:          ctx,
		c:            c,
		identifiers:  toSet(identifiers),
		filterCount:  len(identifiers),
		responseChan: make(chan *CANFrame, bufferSize),
	}
}

func toSet(identifiers []uint32) map[uint32]struct{} {
	idMap := make(map[uint32]struct{}, len(identifiers))
	for _, id := range identifiers {
		idMap[id] = struct{}{}
	}
	return idMap
}
