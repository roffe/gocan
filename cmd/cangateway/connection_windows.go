package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const pipe = `\\.\pipe\gocangateway`

func isRunning() bool {
	conn, err := grpc.NewClient(
		"passthrough:"+pipe,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(winio.DialPipeContext),
	)
	if err != nil {
		log.Println(err)
		return false
	}
	defer conn.Close()
	client := proto.NewGocanClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.SendCommand(ctx, &proto.Command{Data: []byte("ping")})
	if err != nil {
		return false
	}
	if bytes.Equal(resp.GetData(), []byte("pong")) {
		return true
	}
	return false

}

func newListener() (net.Listener, error) {
	return winio.ListenPipe(pipe, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;WD)", // world-read/write; tighten for prod
		InputBufferSize:    1 << 20,           // 1 MiB
		OutputBufferSize:   1 << 20,
	})
}

func cleanup() {
	// No cleanup needed for Windows named pipes
}
