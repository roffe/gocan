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
	SystemMsgDebug
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
	OnError                func(error)
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
	default:
		return errors.New("gocan failed to send frame")
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
	p := c.newSub(ctx, 1, identifiers...)
	c.fh.register <- p
	defer func() {
		c.fh.unregister <- p
	}()
	if err := c.Send(frame); err != nil {
		return nil, err
	}
	return p.Wait(ctx, timeout)
}

// Wait for a certain CAN identifier for up to <timeout>
func (c *Client) Wait(ctx context.Context, timeout time.Duration, identifiers ...uint32) (CANFrame, error) {
	p := c.newSub(ctx, 1, identifiers...)
	c.fh.register <- p
	defer func() {
		c.fh.unregister <- p
	}()
	return p.Wait(ctx, timeout)
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) SubscribeChan(ctx context.Context, identifiers ...uint32) chan CANFrame {
	p := c.newSub(ctx, 10, identifiers...)
	c.fh.register <- p
	return p.callback
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) *Sub {
	p := c.newSub(ctx, 10, identifiers...)
	c.fh.register <- p
	return p
}

func (c *Client) newSub(ctx context.Context, bufferSize int, identifiers ...uint32) *Sub {
	return &Sub{
		ctx:         ctx,
		c:           c,
		identifiers: identifiers,
		filterCount: len(identifiers),
		callback:    make(chan CANFrame, bufferSize),
	}
}
