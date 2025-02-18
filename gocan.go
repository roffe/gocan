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

type Adapter interface {
	Name() string
	Connect(context.Context) error
	Recv() <-chan CANFrame
	Send() chan<- CANFrame
	Err() <-chan error
	Close() error
	//SetFilter([]uint32) error
}

type AdapterConfig struct {
	Debug                  bool
	Port                   string
	PortBaudrate           int
	CANRate                float64
	CANFilter              []uint32
	UseExtendedID          bool
	PrintVersion           bool
	OnMessage              func(string)
	MinimumFirmwareVersion string
}

type Opts func(*Client)

type Client struct {
	fh        *FrameHandler
	adapter   Adapter
	closeOnce sync.Once
}

func NewClient(ctx context.Context, adapter Adapter) (*Client, error) {
	return NewWithOpts(ctx, adapter)
}

func NewWithOpts(ctx context.Context, adapter Adapter, opts ...Opts) (*Client, error) {
	if err := adapter.Connect(ctx); err != nil {
		return nil, err
	}

	c := &Client{
		fh:      newFrameHandler(adapter),
		adapter: adapter,
	}

	for _, opt := range opts {
		opt(c)
	}

	go c.fh.run(ctx)
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
func (c *Client) Send(msg CANFrame) error {
	select {
	case c.adapter.Send() <- msg:
		return nil
	case <-time.After(5 * time.Second):
		// default:
		return errors.New("timeout sending frame (5s)")
	}
}

// Shortcommand to send a standard 11bit frame
func (c *Client) SendFrame(identifier uint32, data []byte, f CANFrameType) error {
	var b = make([]byte, len(data))
	copy(b, data)
	frame := NewFrame(identifier, b, f)
	return c.Send(frame)
}

// Send and wait up to <timeout> for a answer on given identifiers
func (c *Client) SendAndWait(ctx context.Context, frame CANFrame, timeout time.Duration, identifiers ...uint32) (CANFrame, error) {
	frame.SetTimeout(timeout)
	sub := c.newSub(ctx, 1, identifiers...)
	c.fh.register <- sub
	defer func() {
		c.fh.unregister <- sub
	}()
	if err := c.Send(frame); err != nil {
		return nil, err
	}
	return sub.Wait(ctx, timeout)
}

// Wait for a certain CAN identifier for up to <timeout>
func (c *Client) Wait(ctx context.Context, timeout time.Duration, identifiers ...uint32) (CANFrame, error) {
	sub := c.newSub(ctx, 1, identifiers...)
	c.fh.register <- sub
	defer func() {
		c.fh.unregister <- sub
	}()
	return sub.Wait(ctx, timeout)
}

func (c *Client) SubscribeFunc(ctx context.Context, f func(CANFrame), identifiers ...uint32) *Sub {
	sub := c.newSub(ctx, 20, identifiers...)
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
func (c *Client) SubscribeChan(ctx context.Context, channel chan CANFrame, identifiers ...uint32) *Sub {
	sub := c.newSub(ctx, 20, identifiers...)
	sub.responseChan = channel
	c.fh.register <- sub
	return sub
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) *Sub {
	sub := c.newSub(ctx, 20, identifiers...)
	c.fh.register <- sub
	return sub
}

func (c *Client) newSub(ctx context.Context, bufferSize int, identifiers ...uint32) *Sub {
	idMap := make(map[uint32]struct{}, len(identifiers))
	for _, id := range identifiers {
		idMap[id] = struct{}{}
	}
	return &Sub{
		ctx:          ctx,
		c:            c,
		identifiers:  idMap,
		filterCount:  len(identifiers),
		responseChan: make(chan CANFrame, bufferSize),
	}
}
