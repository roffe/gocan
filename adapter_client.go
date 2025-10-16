package gocan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var _ Adapter = (*GWClient)(nil)

type GWClient struct {
	BaseAdapter
	closeOnce sync.Once
	conn      *grpc.ClientConn
}

func NewGWClient(adapterName string, cfg *AdapterConfig) (*GWClient, error) {
	if cfg.OnMessage == nil {
		cfg.OnMessage = func(msg string) {
			_, file, no, ok := runtime.Caller(1)
			if ok {
				fmt.Printf("%s#%d %v\n", filepath.Base(file), no, msg)
			} else {
				log.Println(msg)
			}
		}
	}
	return &GWClient{
		BaseAdapter: NewBaseAdapter(adapterName, cfg),
	}, nil
}

func createStreamMeta(adapterName string, cfg *AdapterConfig) metadata.MD {
	// comma separated list of uint32s as a string c.cfg.CANFilter
	filterIDs := make([]string, 0, len(cfg.CANFilter))
	for _, id := range cfg.CANFilter {
		filterIDs = append(filterIDs, strconv.FormatUint(uint64(id), 10))
	}
	return metadata.Pairs(
		"adapter", adapterName,
		"port", cfg.Port,
		"port_baudrate", strconv.Itoa(cfg.PortBaudrate),
		"canrate", strconv.FormatFloat(cfg.CANRate, 'f', 3, 64),
		"canfilter", strings.Join(filterIDs, ","),
		"debug", strconv.FormatBool(cfg.Debug),
		"useextendedid", strconv.FormatBool(cfg.UseExtendedID),
		"minversion", cfg.MinimumFirmwareVersion,
	)
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

	if !bytes.Equal(initResp.Data, []byte("OK")) {
		log.Printf("init response: %X", initResp.Data)
		return fmt.Errorf("unexpected init response: %s", string(initResp.Data))
	}

	go c.sendManager(ctx, stream)
	go c.recvManager(ctx, stream)

	return nil
}

func (c *GWClient) sendManager(ctx context.Context, stream grpc.BidiStreamingClient[proto.CANFrame, proto.CANFrame]) {
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
				c.SetError(fmt.Errorf("sendManager: %w", err))
				return
			}
		}
	}
}

func (c *GWClient) sendMessage(stream grpc.BidiStreamingClient[proto.CANFrame, proto.CANFrame], msg *CANFrame) error {
	var id uint32 = msg.Identifier
	typ := proto.CANFrameTypeEnum(msg.FrameType.Type)
	resps := uint32(msg.FrameType.Responses)
	frame := &proto.CANFrame{
		Id:   &id,
		Data: msg.Data,
		FrameType: &proto.CANFrameType{
			FrameType: &typ,
			Responses: &resps,
		},
	}
	return stream.Send(frame)
}

func (c *GWClient) recvManager(_ context.Context, stream grpc.BidiStreamingClient[proto.CANFrame, proto.CANFrame]) {
	for {
		in, err := stream.Recv()
		if err != nil {
			if e, ok := status.FromError(err); ok {
				switch e.Code() {
				case codes.Canceled:
					log.Println("client recv canceled")
					return
					//case codes.PermissionDenied:
					//	fmt.Println(e.Message()) // this will print PERMISSION_DENIED_TEST
					//case codes.Internal:
					//	fmt.Println("Has Internal Error")
					//case codes.Aborted:
					//	fmt.Println("gRPC Aborted the call")
					//default:
					//	log.Println(e.Code(), e.Message())
				}
			}

			c.SetError(Unrecoverable(err))
			return
		}

		c.recvMessage(in.GetId(), in.GetData())
	}
}

func (c *GWClient) recvMessage(identifier uint32, data []byte) {

	switch identifier {
	case SystemMsg:
		c.cfg.OnMessage(string(data))
		return
	case SystemMsgError:
		c.SetError(errors.New(string(data)))
		return
	case SystemMsgUnrecoverableError:
		c.SetError(Unrecoverable(errors.New(string(data))))
		return
	}

	frame := NewFrame(identifier, data, Incoming)
	select {
	case c.recvChan <- frame:
	default:
		c.cfg.OnMessage("client recv channel full")
	}
}

func (c *GWClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeChan)
	})
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *GWClient) SetFilter([]uint32) error {
	return nil
}
