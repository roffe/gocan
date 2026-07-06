// Package gateway runs CAN adapters served by a gocangateway instance on
// this machine (gRPC over a local socket / named pipe). Adapters are not
// registered at import time — the gateway reports what it serves at runtime
// (NewGRPCClient + GetAdapters); open one with New(adapterName, cfg).
package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Client struct {
	cfg  gocan.Config
	name string
	bus  *gocan.Bus

	conn   *grpc.ClientConn
	stream grpc.BidiStreamingClient[proto.CANFrame, proto.StreamMessage]
}

// New returns a gateway-served adapter by the name the gateway lists it as.
func New(adapterName string, cfg gocan.Config) (*Client, error) {
	return &Client{cfg: cfg, name: adapterName}, nil
}

// Name reports the gateway-side adapter name (used by Bus.AdapterName).
func (c *Client) Name() string { return c.name }

func (c *Client) Open(ctx context.Context, bus *gocan.Bus) error {
	c.bus = bus
	conn, cl, err := NewGRPCClient()
	if err != nil {
		return fmt.Errorf("could not connect to GoCAN Gateway: %w", err)
	}
	c.conn = conn

	stream, err := cl.Stream(metadata.NewOutgoingContext(ctx, c.streamMeta()))
	if err != nil {
		conn.Close()
		return fmt.Errorf("error opening stream: %w", err)
	}
	initResp, err := stream.Recv()
	if err != nil {
		conn.Close()
		return fmt.Errorf("error receiving init response: %w", err)
	}
	if ev := initResp.GetEvent(); ev == nil || ev.GetMessage() != "OK" {
		conn.Close()
		return fmt.Errorf("unexpected init response: %v", initResp)
	}
	c.stream = stream

	go c.readLoop(ctx)
	return nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *Client) Send(ctx context.Context, f gocan.Frame) error {
	frameType := uint32(1) // v1 Outgoing
	responses := gocan.ExpectedResponses(ctx)
	if responses > 0 {
		frameType = 2 // v1 ResponseRequired
	}
	return c.stream.Send(&proto.CANFrame{
		Id:        f.ID,
		Data:      f.Bytes(),
		FrameType: proto.CANFrameTypeEnum(frameType),
		Responses: uint32(responses),
	})
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		in, err := c.stream.Recv()
		if err != nil {
			if e, ok := status.FromError(err); ok && e.Code() == codes.Canceled {
				return
			}
			if ctx.Err() == nil {
				c.bus.Fatal(err)
			}
			return
		}
		switch p := in.GetPayload().(type) {
		case *proto.StreamMessage_Frame:
			data := p.Frame.GetData()
			if len(data) > 8 {
				data = data[:8]
			}
			f := gocan.Frame{ID: p.Frame.GetId(), Length: uint8(len(data))}
			copy(f.Data[:], data)
			c.bus.Deliver(f)
		case *proto.StreamMessage_Event:
			c.deliverEvent(p.Event)
		}
	}
}

func (c *Client) deliverEvent(e *proto.Event) {
	switch e.GetLevel() {
	case proto.EventLevel_EVENT_FATAL:
		c.bus.Fatal(errors.New(e.GetMessage()))
	case proto.EventLevel_EVENT_ERROR:
		c.bus.Emit(gocan.Event{Type: gocan.EventTypeError, Details: e.GetMessage()})
	case proto.EventLevel_EVENT_WARN:
		c.bus.Emit(gocan.Event{Type: gocan.EventTypeWarning, Details: e.GetMessage()})
	case proto.EventLevel_EVENT_DEBUG:
		c.bus.Emit(gocan.Event{Type: gocan.EventTypeDebug, Details: e.GetMessage()})
	default: // EVENT_INFO
		c.bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: e.GetMessage()})
	}
}

func (c *Client) streamMeta() metadata.MD {
	filterIDs := make([]string, 0, len(c.cfg.CANFilter))
	for _, id := range c.cfg.CANFilter {
		filterIDs = append(filterIDs, strconv.FormatUint(uint64(id), 10))
	}
	md := metadata.Pairs(
		"adapter", c.name,
		"port", c.cfg.Port,
		"port_baudrate", strconv.Itoa(c.cfg.PortBaudrate),
		"canrate", strconv.FormatFloat(c.cfg.CANRate, 'f', 3, 64),
		"canfilter", strings.Join(filterIDs, ","),
		"debug", strconv.FormatBool(c.cfg.Debug),
		"useextendedid", strconv.FormatBool(c.cfg.UseExtendedID),
	)
	if val, found := c.cfg.Extra["minversion"]; found && val != "" {
		md.Append("minversion", val)
	}
	return md
}
