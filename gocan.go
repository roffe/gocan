package gocan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan/pkg/frame"
)

const (
	CR = 0x0D
)

type Adapter interface {
	Init(context.Context) error
	Recv() <-chan frame.CANFrame
	Send() chan<- frame.CANFrame
	Close() error
}

type AdapterConfig struct {
	Port         string
	PortBaudrate int
	CANRate      float64
	CANFilter    []uint32
}

type Client struct {
	hub     *Hub
	adapter Adapter
	send    chan<- frame.CANFrame
}

func New(ctx context.Context, adapter Adapter) (*Client, error) {
	if err := adapter.Init(ctx); err != nil {
		return nil, err
	}
	c := &Client{
		hub:     newHub(adapter.Recv()),
		adapter: adapter,
		send:    adapter.Send(),
	}
	go c.hub.run(ctx)
	return c, nil
}

func (c *Client) Close() error {
	return c.adapter.Close()
}

// Send a CAN Frame
func (c *Client) Send(msg frame.CANFrame) error {
	select {
	case c.send <- msg:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("failed to send frame")
	}
}

// Shortcommand to send a standard 11bit frame
func (c *Client) SendFrame(identifier uint32, data []byte, t frame.CANFrameType) error {
	var b = make([]byte, len(data))
	copy(b, data)
	frame := frame.New(identifier, b, t)
	return c.Send(frame)
}

// SendString is used to bypass the frame parser and send raw commands to the CANUSB adapter
func (c *Client) SendString(str string) error {
	return c.Send(frame.NewRawCommand(str))
}

// Send and wait up to <timeout> for a answer on given identifiers
func (c *Client) SendAndPoll(ctx context.Context, frame *frame.Frame, timeout time.Duration, identifiers ...uint32) (frame.CANFrame, error) {
	frame.SetTimeout(timeout)
	p := newPoller(1, identifiers...)

	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()

	if err := c.Send(frame); err != nil {
		return nil, err
	}

	return waitForFrame(ctx, timeout, p, identifiers...)
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) chan frame.CANFrame {
	p := newPoller(100, identifiers...)
	c.hub.register <- p
	return p.callback
}

// Poll for a certain CAN identifier for up to <timeout>
func (c *Client) Poll(ctx context.Context, timeout time.Duration, identifiers ...uint32) (frame.CANFrame, error) {
	p := newPoller(1, identifiers...)
	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()
	return waitForFrame(ctx, timeout, p, identifiers...)
}

func waitForFrame(ctx context.Context, timeout time.Duration, p *Poll, identifiers ...uint32) (frame.CANFrame, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f := <-p.callback:
		if f == nil {
			return nil, errors.New("got nil frame from poller")
		}
		return f, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for frame 0x%03X", identifiers)

	}
}
