package gocan

import (
	"context"

	"github.com/roffe/gocan/pkg/model"
)

const (
	CR = 0x0D
)

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
func (c *Client) Send(msg model.CANFrame) error {
	return c.device.Send(msg)
}

// Shortcommand to send a standard 11bit frame
func (c *Client) SendFrame(identifier uint32, data []byte, opts ...model.FrameOpt) error {
	var b = make([]byte, 8)
	copy(b, data)

	frame := model.NewFrame(identifier, b, model.OutResponseRequired)

	for _, o := range opts {
		o(frame)
	}

	return c.Send(frame)
}

// SendString is used to bypass the frame parser and send raw commands to the CANUSB adapter
func (c *Client) SendString(str string) error {
	return c.Send(model.NewRawCommand(str))
}
