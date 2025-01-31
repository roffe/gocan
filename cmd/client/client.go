package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/client"
	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	globalCtx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down client")
		cancel()
	}()

	adapter := client.New("txbridge", &gocan.AdapterConfig{
		Port:                   "COM4",
		PortBaudrate:           2000000,
		CANRate:                500,
		CANFilter:              []uint32{0x1A0},
		MinimumFirmwareVersion: "1.0.6",
	})

	c, err := gocan.New(globalCtx, adapter)
	if err != nil {
		log.Fatalf("could not create client: %v", err)
	}
	defer c.Close()

	sub := c.Subscribe(globalCtx)
	defer sub.Close()

	in := sub.C()

	for {
		select {
		case frame := <-in:
			log.Println(frame.String())
		case <-globalCtx.Done():
			return
		}
	}
}

func main2() {
	globalCtx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down client")
		cancel()
	}()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("Failed to get user cache dir: %v", err)
	}

	socketFile := filepath.Join(cacheDir, "gocan.sock")

	conn, err := grpc.NewClient("unix:"+socketFile, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := proto.NewGocanClient(conn)

	adapters, err := c.GetAdapters(context.Background(), &emptypb.Empty{})
	if err != nil {
		log.Fatalf("could not get adapters: %v", err)
	}

	log.Println("Adapters:")
	for _, a := range adapters.Adapters {

		log.Printf("  %s", *a.Name)
		log.Printf("  %s", *a.Description)
		log.Printf("  %v", a.Capabilities)
		log.Printf("  %v", *a.RequireSerialPort)
		log.Println(" " + strings.Repeat("-", 40))
	}

	meta := metadata.Pairs(
		"adapter", "txbridge",
		"port", "COM4",
		"port_baudrate", "2000000",
	)

	ctx := metadata.NewOutgoingContext(globalCtx, meta)

	stream, err := c.Stream(ctx)
	if err != nil {
		log.Fatalf("could not stream: %v", err)
	}

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				var id uint32 = 0x123
				if err := stream.Send(&proto.CANFrame{Id: &id, Data: []byte{0x01, 0x02, 0x03, 0x04}}); err != nil {
					log.Fatalf("could not send: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		in, err := stream.Recv()
		if err != nil {
			log.Fatalf("could not receive: %v", err)
		}
		frame := gocan.NewFrame(*in.Id, in.Data, gocan.Incoming)
		log.Println(frame.ColorString())
	}

}
