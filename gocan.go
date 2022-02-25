package gocan

import (
	"context"
	"time"

	"github.com/roffe/gocan/pkg/frame"
)

const (
	CR = 0x0D
)

type Adapter interface {
	Init(context.Context) error
	SetPort(string) error
	SetPortRate(int) error
	SetCANrate(float64) error
	SetCANfilter(...uint32)
	Chan() <-chan frame.CANFrame
	Send(frame.CANFrame) error
	Close() error
}

type Client struct {
	hub    *Hub
	device Adapter
}

func New(ctx context.Context, device Adapter, filters []uint32) (*Client, error) {
	c := &Client{
		hub:    newHub(device.Chan()),
		device: device,
	}
	go c.hub.run(ctx)
	return c, nil
}

func (c *Client) Close() error {
	return c.device.Close()
}

// Send a CAN Frame
func (c *Client) Send(msg frame.CANFrame) error {
	return c.device.Send(msg)
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

// Send and wait up to <timeout> for a answer
func (c *Client) SendAndPoll(ctx context.Context, frame *frame.Frame, timeout time.Duration, identifiers ...uint32) (frame.CANFrame, error) {
	frame.SetTimeout(timeout)
	p := newPoller(1, identifiers...)

	c.hub.register <- p
	defer func() {
		c.hub.unregister <- p
	}()

	if err := c.device.Send(frame); err != nil {
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
