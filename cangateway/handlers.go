package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/proto"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func adapterConfigFromContext(ctx context.Context) (string, *gocan.AdapterConfig, error) {
	md, exists := metadata.FromIncomingContext(ctx)
	if !exists {
		return "", nil, errors.New("connect metadata not found")
	}
	dbg, err := strconv.ParseBool(md["debug"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid debug: %w", err)
	}
	portBaudrate, err := strconv.Atoi(md["port_baudrate"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid port_baudrate: %w", err)
	}
	canrate, err := strconv.ParseFloat(md["canrate"][0], 64)
	if err != nil {
		return "", nil, fmt.Errorf("invalid canrate: %w", err)
	}
	useExtendedID, err := strconv.ParseBool(md["useextendedid"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid useextendedid: %w", err)
	}
	return md["adapter"][0], &gocan.AdapterConfig{
		Debug:                  dbg,
		Port:                   md["port"][0],
		PortBaudrate:           portBaudrate,
		CANRate:                canrate,
		CANFilter:              parseFilters(strings.Split(md["canfilter"][0], ",")),
		UseExtendedID:          useExtendedID,
		MinimumFirmwareVersion: md["minversion"][0],
		PrintVersion:           true,
	}, nil
}

func parseFilters(filters []string) []uint32 {
	var canfilters []uint32
	for _, id := range filters {
		i, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			log.Printf("invalid canfilter: %v", err)
			continue
		}
		canfilters = append(canfilters, uint32(i))
	}
	return canfilters
}

func send(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame], id uint32, data []byte) error {
	frameTyp := proto.CANFrameTypeEnum_Incoming
	return srv.Send(&proto.CANFrame{
		Id:   &id,
		Data: data,
		FrameType: &proto.CANFrameType{
			FrameType: &frameTyp,
			Responses: new(uint32),
		},
	})
}

func (s *Server) SendCommand(ctx context.Context, in *proto.Command) (*proto.CommandResponse, error) {
	switch {
	case bytes.Equal(in.GetData(), []byte("ping")):
		return &proto.CommandResponse{Data: []byte("pong")}, nil
	case bytes.Equal(in.GetData(), []byte("quit")):
		go func() {
			time.Sleep(5 * time.Millisecond)
			if err := s.Close(); err != nil {
				log.Fatalf("failed to close server: %v", err)
			}
		}()
		return &proto.CommandResponse{Data: []byte("OK")}, nil
	default:
		return nil, fmt.Errorf("unknown command: %s", in.GetData())
	}
}

func (s *Server) Stream(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame]) error {
	// gctx, cancel := context.WithCancel(srv.Context())
	gctx := srv.Context()

	adaptername, adapterConfig, err := adapterConfigFromContext(gctx)
	if err != nil {
		return fmt.Errorf("failed to create adapter config: %w", err)
	}

	adapterConfig.OnError = func(err error) {
		send(srv, gocan.SystemMsgError, []byte(err.Error()))
		log.Printf("adapter error: %v", err)
		_, file, no, ok := runtime.Caller(1)
		if ok {
			fmt.Printf("%s#%d %v\n", file, no, err)
		} else {
			log.Println(err)
		}
	}

	adapterConfig.OnMessage = func(s string) {
		_, file, no, ok := runtime.Caller(1)
		if ok {
			fmt.Printf("%s#%d %v\n", file, no, s)
		} else {
			log.Println(s)
		}
	}

	dev, err := adapter.New(adaptername, adapterConfig)
	if err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}

	errg, ctx := errgroup.WithContext(gctx)

	if err := dev.Connect(ctx); err != nil {
		send(srv, 0, []byte(err.Error()))
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer dev.Close()
	log.Printf("%s connected @ %g kbp/s", adaptername, adapterConfig.CANRate)
	defer log.Printf("%s disconnected", adaptername)
	send(srv, 0, []byte("OK"))
	// send mesage from canbus adapter to IPC
	errg.Go(s.recvManager(ctx, srv, dev))
	// send message from IPC to canbus adapter
	go s.sendManager(srv, dev)()

	if err := errg.Wait(); err != nil {
		if err == context.Canceled {
			return nil
		}
		log.Println("stream error:", err)
		return err
	}
	return nil
}

func (s *Server) recvManager(ctx context.Context, srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame], dev gocan.Adapter) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-dev.Err():
				send(srv, gocan.SystemMsgError, []byte(err.Error()))
				return fmt.Errorf("adapter error: %w", err)
			case msg, ok := <-dev.Recv():
				if !ok {
					return errors.New("adapter recv channel closed")
				}
				if msg == nil {
					log.Println("adapter nil message")
					continue
				}
				if err := s.recvMessage(srv, msg); err != nil {
					return err
				}
			}
		}
	}
}

func (s *Server) recvMessage(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame], msg gocan.CANFrame) error {
	id := msg.Identifier()
	frameType := proto.CANFrameTypeEnum(msg.Type().Type)
	responseCount := uint32(msg.Type().Responses)
	//log.Println("frameTyp:", frameTyp)
	//log.Println("responses:", responses)
	mmsg := &proto.CANFrame{
		Id:   &id,
		Data: msg.Data(),
		FrameType: &proto.CANFrameType{
			FrameType: &frameType,
			Responses: &responseCount,
		},
	}
	if err := srv.Send(mmsg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func (s *Server) sendManager(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame], dev gocan.Adapter) func() error {
	return func() error {
		for {
			msg, err := srv.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					log.Println("client closed connection")
					return nil // Client closed connection
				}
				return fmt.Errorf("failed to receive outgoing %w", err) // Something unexpected happened
			}
			s.sendMessage(srv, dev, msg)
		}
	}
}

func (s *Server) sendMessage(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame], dev gocan.Adapter, msg *proto.CANFrame) {
	t := msg.GetFrameType()
	frame := gocan.NewFrame(*msg.Id, msg.Data, gocan.CANFrameType{
		Type:      int(t.GetFrameType()),
		Responses: int(t.GetResponses()),
	})
	select {
	case dev.Send() <- frame:
	default:
		send(srv, gocan.SystemMsgError, []byte("adapter send buffer full"))
	}
}

func (s *Server) GetAdapters(ctx context.Context, _ *emptypb.Empty) (*proto.Adapters, error) {
	//md, _ := metadata.FromIncomingContext(ctx)
	//for k, v := range md {
	//	log.Printf("metadata: %s: %v", k, v)
	//}
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
	return nil, nil
}
