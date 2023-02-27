package gocan

import (
	"context"
	"errors"
	"time"
)

const (
	CR = 0x0D
)

type Adapter interface {
	Init(context.Context) error
	Name() string
	Recv() <-chan CANFrame
	Send() chan<- CANFrame
	Close() error
}

type AdapterConfig struct {
	Port         string
	PortBaudrate int
	CANRate      float64
	CANFilter    []uint32
	Output       func(string)
}

type Client struct {
	fh      *FrameHandler
	adapter Adapter
}

func New(ctx context.Context, adapter Adapter) (*Client, error) {
	if err := adapter.Init(ctx); err != nil {
		return nil, err
	}
	c := &Client{
		fh:      newFrameHandler(adapter.Recv()),
		adapter: adapter,
	}
	go c.fh.run(ctx)
	return c, nil
}

func (c *Client) Adapter() Adapter {
	return c.adapter
}

func (c *Client) Close() error {
	c.fh.Close()
	return c.adapter.Close()
}

// Send a CAN Frame
func (c *Client) Send(msg CANFrame) error {
	select {
	case c.adapter.Send() <- msg:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("failed to send frame")
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
func (c *Client) SendAndPoll(ctx context.Context, frame CANFrame, timeout time.Duration, identifiers ...uint32) (CANFrame, error) {
	frame.SetTimeout(timeout)
	p := newSub(1, identifiers...)

	c.fh.register <- p
	defer func() {
		c.fh.unregister <- p
	}()

	if err := c.Send(frame); err != nil {
		return nil, err
	}

	return p.Wait(ctx, timeout)
}

// Poll for a certain CAN identifier for up to <timeout>
func (c *Client) Poll(ctx context.Context, timeout time.Duration, identifiers ...uint32) (CANFrame, error) {
	p := newSub(1, identifiers...)
	c.fh.register <- p
	defer func() {
		c.fh.unregister <- p
	}()
	return p.Wait(ctx, timeout)
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) chan CANFrame {
	p := newSub(10, identifiers...)
	c.fh.register <- p
	return p.callback
}
