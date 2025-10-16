package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var socketFile = filepath.Join(os.TempDir(), "cangateway.sock")

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}

func isRunning() bool {
	if fileExists(socketFile) {
		conn, err := grpc.NewClient(
			"unix:"+socketFile,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			log.Println(err)
			os.Remove(socketFile)
			return false
		}
		defer conn.Close()
		client := proto.NewGocanClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.SendCommand(ctx, &proto.Command{Data: []byte("ping")})
		if err != nil {
			os.Remove(socketFile)
			return false
		}
		if bytes.Equal(resp.GetData(), []byte("pong")) {
			return true
		}
		os.Remove(socketFile)
		return false
	}
	return false
}

func newListener() (net.Listener, error) {
	return net.Listen("unix", socketFile)
}

func cleanup() {
	if fileExists(socketFile) {
		os.Remove(socketFile)
	}
}
