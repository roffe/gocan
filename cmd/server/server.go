package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/proto"
	"go.bug.st/serial/enumerator"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("failed to get user cache dir: %v", err)
	}

	// Start IPC server
	socketFile := filepath.Join(cacheDir, "gocan.sock")

	log.Printf("Starting GoCAN IPC server 1.0.0")

	srv := NewServer(socketFile)

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server")
		if err := srv.Close(); err != nil {
			log.Fatalf("failed to close server: %v", err)
		}
	}()

	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

var _ proto.GocanServer = (*Server)(nil)

type Server struct {
	proto.UnimplementedGocanServer

	l       net.Listener
	clients sync.Map
}

func NewServer(socketFile string) *Server {
	l, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Fatal(err)
	}
	srv := &Server{l: l}

	return srv
}

func (s *Server) Run() error {
	sg := grpc.NewServer()
	proto.RegisterGocanServer(sg, s)
	log.Printf("server listening at %v", s.l.Addr())
	if err := sg.Serve(s.l); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

func (s *Server) Close() error {
	return s.l.Close()
}

func adapterConfigFromContext(ctx context.Context) (string, *gocan.AdapterConfig, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	for k, v := range md {
		log.Printf("metadata: %s: %v", k, v)
	}

	adaptername := md["adapter"][0]
	adapterPort := md["port"][0]
	portBaudrate, err := strconv.Atoi(md["port_baudrate"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid port_baudrate: %w", err)
	}

	canrate, err := strconv.ParseFloat(md["canrate"][0], 64)
	if err != nil {
		return "", nil, fmt.Errorf("invalid canrate: %w", err)
	}

	filterIDs := strings.Split(md["canfilter"][0], ",")

	var canfilters []uint32
	for _, id := range filterIDs {
		i, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			return "", nil, fmt.Errorf("invalid canfilter: %w", err)
		}
		canfilters = append(canfilters, uint32(i))
	}

	useExtendedID, err := strconv.ParseBool(md["useextendedid"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid useextendedid: %w", err)
	}

	minversion := md["minversion"][0]

	return adaptername, &gocan.AdapterConfig{
		Port:                   adapterPort,
		PortBaudrate:           portBaudrate,
		CANRate:                canrate,
		CANFilter:              canfilters,
		UseExtendedID:          useExtendedID,
		MinimumFirmwareVersion: minversion,
	}, nil
}

func (s *Server) Stream(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame]) error {
	// gctx, cancel := context.WithCancel(srv.Context())
	gctx := srv.Context()

	adaptername, adapterConfig, err := adapterConfigFromContext(gctx)
	if err != nil {
		return fmt.Errorf("failed to create adapter config: %w", err)
	}

	adapterConfig.OnError = func(err error) {
		log.Printf("adapter error: %v", err)
	}

	adapterConfig.OnMessage = func(s string) {
		log.Printf("adapter message: %v", s)
	}

	dev, err := adapter.New(adaptername, adapterConfig)
	if err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}

	errg, ctx := errgroup.WithContext(gctx)

	client, err := gocan.New(ctx, dev)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	canRX := client.SubscribeChan(ctx)

	// send mesage from canbus adapter to IPC
	errg.Go(func() error {
		for {
			select {
			case msg, ok := <-canRX:
				if !ok {
					return errors.New("canRX closed")
				}
				id := msg.Identifier()
				frameTyp := proto.CANFrameTypeEnum(msg.Type().Type)
				responses := uint32(msg.Type().Responses)
				frameType := &proto.CANFrameType{
					FrameType: &frameTyp,
					Responses: &responses,
				}
				mmsg := &proto.CANFrame{
					Id:        &id,
					Data:      msg.Data(),
					FrameType: frameType,
				}
				if err := srv.Send(mmsg); err != nil {
					return fmt.Errorf("failed to send message: %w", err)
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	// send message from IPC to canbus adapter
	errg.Go(func() error {
		for {
			msg, err := srv.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil // Client closed connection
				}
				return fmt.Errorf("failed to receive outgoing %w", err) // Something unexpected happened
			}
			r := gocan.CANFrameType{
				Type:      int(*msg.FrameType.FrameType),
				Responses: int(*msg.FrameType.Responses),
			}
			frame := gocan.NewFrame(*msg.Id, msg.Data, r)
			if err := client.Send(frame); err != nil {
				return err
			}
		}
	})

	log.Println("stream waiting")
	err = errg.Wait()
	log.Println("stream done", err)
	return err
}

func (s *Server) GetAdapters(ctx context.Context, _ *emptypb.Empty) (*proto.Adapters, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	for k, v := range md {
		log.Printf("metadata: %s: %v", k, v)
	}
	var adapters []*proto.AdapterInfo
	for _, a := range adapter.GetAdapterMap() {
		adapter := &proto.AdapterInfo{
			Name:        &a.Name,
			Description: &a.Description,
			Capabilities: &proto.AdapterCapabilities{
				HSCAN: &a.Capabilities.HSCAN,
				KLine: &a.Capabilities.KLine,
				SWCAN: &a.Capabilities.SWCAN,
			},
			RequireSerialPort: &a.RequiresSerialPort,
		}
		adapters = append(adapters, adapter)
	}
	return &proto.Adapters{
		Adapters: adapters,
	}, nil
}

func (s *Server) GetSerialPorts(ctx context.Context, _ *emptypb.Empty) (*proto.SerialPorts, error) {
	var portsList []string
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		//m.output(err.Error())
		return []string{}
	}
	if len(ports) == 0 {
		//m.output("No serial ports found!")
		return []string{}
	}

	for _, port := range ports {
		//m.output(fmt.Sprintf("Found port: %s", port.Name))
		//if port.IsUSB {
		//m.output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
		//m.output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
		portsList = append(portsList, port.Name)
		//}
	}

	sort.Strings(portsList)
}
