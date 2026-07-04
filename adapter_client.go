package gocan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var _ Adapter = (*GWClient)(nil)

type GWClient struct {
	*BaseAdapter
	conn *grpc.ClientConn
}

func NewGWClient(adapterName string, cfg *AdapterConfig) (*GWClient, error) {
	return &GWClient{
		BaseAdapter: NewSyncBaseAdapter(adapterName, cfg),
	}, nil
}

func createStreamMeta(adapterName string, cfg *AdapterConfig) metadata.MD {
	// comma separated list of uint32s as a string c.cfg.CANFilter
	filterIDs := make([]string, 0, len(cfg.CANFilter))
	for _, id := range cfg.CANFilter {
		filterIDs = append(filterIDs, strconv.FormatUint(uint64(id), 10))
	}
	md := metadata.Pairs(
		"adapter", adapterName,
		"port", cfg.Port,
		"port_baudrate", strconv.Itoa(cfg.PortBaudrate),
		"canrate", strconv.FormatFloat(cfg.CANRate, 'f', 3, 64),
		"canfilter", strings.Join(filterIDs, ","),
		"debug", strconv.FormatBool(cfg.Debug),
		"useextendedid", strconv.FormatBool(cfg.UseExtendedID),
	)
	if val, found := cfg.AdditionalConfig["minversion"]; found && val != "" {
		md.Append("minversion", val)
	}
	return md
}

func (c *GWClient) Open(gctx context.Context) error {
	conn, cl, err := NewGRPCClient()
	if err != nil {
		return fmt.Errorf("could not connect to GoCAN Gateway: %w", err)
	}
	c.conn = conn

	ctx := metadata.NewOutgoingContext(gctx, createStreamMeta(c.name, c.cfg))

	stream, err := cl.Stream(ctx)
	if err != nil {
		return fmt.Errorf("error opening stream: %w", err)
	}

	initResp, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("error receiving init response: %w", err)
	}

	if ev := initResp.GetEvent(); ev == nil || ev.GetMessage() != "OK" {
		return fmt.Errorf("unexpected init response: %v", initResp)
	}

	go c.sendManager(ctx, stream)
	go c.recvManager(ctx, stream)

	return nil
}

func (c *GWClient) sendManager(ctx context.Context, stream grpc.BidiStreamingClient[proto.CANFrame, proto.StreamMessage]) {
	if c.cfg.Debug {
		log.Println("sendManager started")
		defer log.Println("sendManager done")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeChan:
			return
		case msg := <-c.sendChan:
			if err := c.sendMessage(stream, msg); err != nil {
				c.Fatal(fmt.Errorf("sendManager: %w", err))
				return
			}
		}
	}
}

func (c *GWClient) sendMessage(stream grpc.BidiStreamingClient[proto.CANFrame, proto.StreamMessage], msg *CANFrame) error {
	defer msg.markSent()
	return stream.Send(&proto.CANFrame{
		Id:        msg.Identifier,
		Data:      msg.Data,
		FrameType: proto.CANFrameTypeEnum(msg.FrameType.Type),
		Responses: uint32(msg.FrameType.Responses),
	})
}

func (c *GWClient) recvManager(_ context.Context, stream grpc.BidiStreamingClient[proto.CANFrame, proto.StreamMessage]) {
	for {
		in, err := stream.Recv()
		if err != nil {
			if e, ok := status.FromError(err); ok {
				switch e.Code() {
				case codes.Canceled:
					log.Println("client recv canceled")
					return
				}
			}

			c.Fatal(err)
			return
		}

		switch p := in.GetPayload().(type) {
		case *proto.StreamMessage_Frame:
			c.deliverFrame(p.Frame)
		case *proto.StreamMessage_Event:
			c.deliverEvent(p.Event)
		}
	}
}

func (c *GWClient) deliverFrame(f *proto.CANFrame) {
	frame := NewFrame(f.GetId(), f.GetData(), Incoming)
	select {
	case c.recvChan <- frame:
	default:
		c.Error(ErrDroppedFrame)
	}
}

func (c *GWClient) deliverEvent(e *proto.Event) {
	switch e.GetLevel() {
	case proto.EventLevel_EVENT_FATAL:
		c.Fatal(errors.New(e.GetMessage()))
	case proto.EventLevel_EVENT_ERROR:
		c.sendEvent(EventTypeError, e.GetMessage())
	case proto.EventLevel_EVENT_WARN:
		c.Warn(e.GetMessage())
	case proto.EventLevel_EVENT_DEBUG:
		c.Debug(e.GetMessage())
	default: // EVENT_INFO
		c.Info(e.GetMessage())
	}
}

func (c *GWClient) Close() error {
	c.BaseAdapter.Close()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *GWClient) SetFilter([]uint32) error {
	return nil
}
