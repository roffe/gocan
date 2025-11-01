//go:generate protoc.exe --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative .\proto\server.proto

package gocan

import (
	"context"
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

type Opt func(*Client)

type Client struct {
	fh        *handler
	adapter   Adapter
	closeOnce sync.Once
}

func WithEventHandler(fn func(Event)) Opt {
	return func(c *Client) {
		c.fh.setOnEvent(fn)
	}
}

// Create a new CAN client with given adapter name and config
func New(ctx context.Context, adapterName string, cfg *AdapterConfig) (*Client, error) {
	adapter, err := NewAdapter(adapterName, cfg)
	if err != nil {
		return nil, err
	}
	return NewWithOpts(ctx, adapter)
}

// Create a new CAN client with given adapter and options
func NewWithOpts(ctx context.Context, adapter Adapter, opts ...Opt) (*Client, error) {
	c := &Client{
		fh:      newHandler(adapter),
		adapter: adapter,
	}
	for _, opt := range opts {
		opt(c)
	}
	go c.fh.run(ctx)
	return c, adapter.Open(ctx)
}

// Return the underlying adapter
func (c *Client) Adapter() Adapter {
	return c.adapter
}

// Return the name of the underlying adapter
func (c *Client) AdapterName() string {
	return c.adapter.Name()
}

// Wait for the first critical error or closure of the client
func (c *Client) Wait() error {
	return <-c.adapter.Err()
}

func (c *Client) Event() <-chan Event {
	return c.adapter.Event()
}

// Close the client and underlying adapter
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.adapter.Close()
		c.fh.close()
	})
	return err
}

// Send a CAN Frame
func (c *Client) SendFrame(msg *CANFrame) error {
	select {
	case c.adapter.Send() <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return &TimeoutError{
			Timeout: 5,
			Frames:  []uint32{msg.Identifier},
			Type:    "send",
		}
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
	frame.Timeout = uint32(timeout.Milliseconds())
	sub, err := c.newSub(1, identifiers...)
	if err != nil {
		return nil, err
	}
	defer func() {
		c.fh.unregisterSubscriber(sub)
	}()
	if err := c.SendFrame(frame); err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sub.wait(waitCtx)
}

// Receive a single CAN frame with specific identifier with a max waiting timeout
func (c *Client) Recv(ctx context.Context, timeout time.Duration, identifiers ...uint32) (*CANFrame, error) {
	sub, err := c.newSub(1, identifiers...)
	if err != nil {
		return nil, err
	}
	defer func() {
		c.fh.unregisterSubscriber(sub)
	}()

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sub.wait(waitCtx)
}

// Subscribe to CAN identifiers with a callback function
func (c *Client) SubscribeFunc(ctx context.Context, fn func(*CANFrame), identifiers ...uint32) *Subscriber {
	sub := c.Subscribe(ctx, identifiers...)
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-sub.responseChan:
				if !ok {
					return
				}
				fn(frame)
			}
		}
	}()
	return sub
}

// Subscribe to CAN identifiers with provided channel
func (c *Client) SubscribeChan(ctx context.Context, channel chan *CANFrame, identifiers ...uint32) *Subscriber {
	sub := &Subscriber{
		cl:           c,
		identifiers:  toSet(identifiers),
		filterCount:  len(identifiers),
		responseChan: channel,
	}
	c.fh.registerSubscriber(sub)
	return sub
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) *Subscriber {
	sub, err := c.newSub(40, identifiers...)
	if err != nil {
		panic(err) // Should never happen
	}
	return sub
}

func (c *Client) newSub(bufferSize int, identifiers ...uint32) (*Subscriber, error) {
	sub := &Subscriber{
		cl:           c,
		identifiers:  toSet(identifiers),
		filterCount:  len(identifiers),
		responseChan: make(chan *CANFrame, bufferSize),
	}
	c.fh.registerSubscriber(sub)
	return sub, nil
}

func toSet(identifiers []uint32) map[uint32]struct{} {
	idMap := make(map[uint32]struct{}, len(identifiers))
	for _, id := range identifiers {
		idMap[id] = struct{}{}
	}
	return idMap
}
