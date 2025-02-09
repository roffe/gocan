package adapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

var (
	_          gocan.Adapter = (*Client)(nil)
	socketFile string
	kacp       = keepalive.ClientParameters{
		Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
		Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
		PermitWithoutStream: true,             // send pings even without active streams
	}
)

func init() {
	if cacheDir, err := os.UserCacheDir(); err == nil {
		socketFile = filepath.Join(cacheDir, "gocan.sock")
	}
}

type Client struct {
	BaseAdapter
	adapterName string
	closeOnce   sync.Once
	conn        *grpc.ClientConn
}

func NewClient(adapterName string, cfg *gocan.AdapterConfig) (*Client, error) {
	if cfg.OnError == nil {
		cfg.OnError = func(err error) {
			_, file, no, ok := runtime.Caller(1)
			if ok {
				fmt.Printf("%s#%d %v\n", file, no, err)
			} else {
				log.Println(err)
			}
		}
	}
	if cfg.OnMessage == nil {
		cfg.OnMessage = func(msg string) {
			_, file, no, ok := runtime.Caller(1)
			if ok {
				fmt.Printf("%s#%d %v\n", file, no, msg)
			} else {
				log.Println(msg)
			}
		}
	}
	return &Client{
		adapterName: adapterName,
		BaseAdapter: NewBaseAdapter(cfg),
	}, nil
}

func createStreamMeta(adapterName string, cfg *gocan.AdapterConfig) metadata.MD {
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

func NewGRPCClient() (*grpc.ClientConn, proto.GocanClient, error) {
	conn, err := grpc.NewClient(
		"unix:"+socketFile,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kacp),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, proto.NewGocanClient(conn), nil
}

func (c *Client) Init(gctx context.Context) error {
	conn, cl, err := NewGRPCClient()
	if err != nil {
		return fmt.Errorf("could not connect to GoCAN Gateway: %w", err)
	}
	c.conn = conn

	ctx := metadata.NewOutgoingContext(gctx, createStreamMeta(c.adapterName, c.cfg))

	stream, err := cl.Stream(ctx)
	if err != nil {
		return fmt.Errorf("error opening stream: %w", err)
	}

	initResp, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("error receiving init response: %w", err)
	}

	if !bytes.Equal(initResp.Data, []byte("OK")) {
		return fmt.Errorf("unexpected init response: %s", string(initResp.Data))
	}

	go c.sendManager(ctx, stream)
	go c.recvManager(ctx, stream)

	return nil
}

func (c *Client) sendManager(ctx context.Context, stream grpc.BidiStreamingClient[proto.CANFrame, proto.CANFrame]) {
	for {
		select {
		case <-c.close:
			log.Println("closing client")
			return
		case msg := <-c.send:
			var id uint32 = msg.Identifier()
			typ := proto.CANFrameTypeEnum(msg.Type().Type)
			resps := uint32(msg.Type().Responses)
			frame := &proto.CANFrame{
				Id:   &id,
				Data: msg.Data(),
				FrameType: &proto.CANFrameType{
					FrameType: &typ,
					Responses: &resps,
				},
			}
			if err := stream.Send(frame); err != nil {
				c.err <- fmt.Errorf("could not send: %w", err)
				return
			}
		case <-ctx.Done():
			close(c.send)
			return
		}
	}

}

func (c *Client) recvManager(ctx context.Context, stream grpc.BidiStreamingClient[proto.CANFrame, proto.CANFrame]) {
	for {
		in, err := stream.Recv()
		if err != nil {
			//if e, ok := status.FromError(err); ok {
			//	switch e.Code() {
			//	case codes.Canceled:
			//		return
			//	//case codes.PermissionDenied:
			//	//	fmt.Println(e.Message()) // this will print PERMISSION_DENIED_TEST
			//	//case codes.Internal:
			//	//	fmt.Println("Has Internal Error")
			//	//case codes.Aborted:
			//	//	fmt.Println("gRPC Aborted the call")
			//	default:
			//		log.Println(e.Code(), e.Message())
			//	}
			//}
			c.err <- fmt.Errorf("could not receive: %w", err)
			log.Println("recv error:", err)
			return
		}
		switch in.GetId() {
		case 0:
			c.err <- errors.New(string(in.GetData()))
			return
		//case SystemMsgError:
		//	c.cfg.OnError(errors.New(string(in.GetData())))
		//	continue
		default:
			frame := gocan.NewFrame(in.GetId(), in.GetData(), gocan.Incoming)
			select {
			case c.recv <- frame:
			default:
				c.cfg.OnError(errors.New("recv channel full"))
			}
		}

	}
}

func (c *Client) Close() (err error) {
	c.closeOnce.Do(func() {
		close(c.close)
	})
	if c.conn != nil {
		err = c.conn.Close()
	}
	return
}

func (c *Client) Name() string {
	return c.adapterName
}

func (c *Client) SetFilter([]uint32) error {
	return nil
}
