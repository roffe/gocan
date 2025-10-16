package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

var _ proto.GocanServer = (*Server)(nil)

type Server struct {
	proto.UnimplementedGocanServer
	l net.Listener
}

func NewServer() *Server {
	l, err := newListener()

	if err != nil {
		log.Fatal(err)
	}
	srv := &Server{l: l}

	return srv
}

var kaep = keepalive.EnforcementPolicy{
	MinTime:             5 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      0,                // If any connection is alive for more than 30 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               3 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
}

func (s *Server) Run() error {
	sg := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp))
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
