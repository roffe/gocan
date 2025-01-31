// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.28.0
// source: proto/server.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Gocan_SendCommand_FullMethodName    = "/Gocan/SendCommand"
	Gocan_GetSerialPorts_FullMethodName = "/Gocan/GetSerialPorts"
	Gocan_GetAdapters_FullMethodName    = "/Gocan/GetAdapters"
	Gocan_Stream_FullMethodName         = "/Gocan/Stream"
)

// GocanClient is the client API for Gocan service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type GocanClient interface {
	SendCommand(ctx context.Context, in *Command, opts ...grpc.CallOption) (*CommandResponse, error)
	GetSerialPorts(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*SerialPorts, error)
	GetAdapters(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*Adapters, error)
	Stream(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[CANFrame, CANFrame], error)
}

type gocanClient struct {
	cc grpc.ClientConnInterface
}

func NewGocanClient(cc grpc.ClientConnInterface) GocanClient {
	return &gocanClient{cc}
}

func (c *gocanClient) SendCommand(ctx context.Context, in *Command, opts ...grpc.CallOption) (*CommandResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CommandResponse)
	err := c.cc.Invoke(ctx, Gocan_SendCommand_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gocanClient) GetSerialPorts(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*SerialPorts, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SerialPorts)
	err := c.cc.Invoke(ctx, Gocan_GetSerialPorts_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gocanClient) GetAdapters(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*Adapters, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Adapters)
	err := c.cc.Invoke(ctx, Gocan_GetAdapters_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gocanClient) Stream(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[CANFrame, CANFrame], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Gocan_ServiceDesc.Streams[0], Gocan_Stream_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[CANFrame, CANFrame]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Gocan_StreamClient = grpc.BidiStreamingClient[CANFrame, CANFrame]

// GocanServer is the server API for Gocan service.
// All implementations must embed UnimplementedGocanServer
// for forward compatibility.
type GocanServer interface {
	SendCommand(context.Context, *Command) (*CommandResponse, error)
	GetSerialPorts(context.Context, *emptypb.Empty) (*SerialPorts, error)
	GetAdapters(context.Context, *emptypb.Empty) (*Adapters, error)
	Stream(grpc.BidiStreamingServer[CANFrame, CANFrame]) error
	mustEmbedUnimplementedGocanServer()
}

// UnimplementedGocanServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedGocanServer struct{}

func (UnimplementedGocanServer) SendCommand(context.Context, *Command) (*CommandResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendCommand not implemented")
}
func (UnimplementedGocanServer) GetSerialPorts(context.Context, *emptypb.Empty) (*SerialPorts, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSerialPorts not implemented")
}
func (UnimplementedGocanServer) GetAdapters(context.Context, *emptypb.Empty) (*Adapters, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAdapters not implemented")
}
func (UnimplementedGocanServer) Stream(grpc.BidiStreamingServer[CANFrame, CANFrame]) error {
	return status.Errorf(codes.Unimplemented, "method Stream not implemented")
}
func (UnimplementedGocanServer) mustEmbedUnimplementedGocanServer() {}
func (UnimplementedGocanServer) testEmbeddedByValue()               {}

// UnsafeGocanServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to GocanServer will
// result in compilation errors.
type UnsafeGocanServer interface {
	mustEmbedUnimplementedGocanServer()
}

func RegisterGocanServer(s grpc.ServiceRegistrar, srv GocanServer) {
	// If the following call pancis, it indicates UnimplementedGocanServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Gocan_ServiceDesc, srv)
}

func _Gocan_SendCommand_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Command)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GocanServer).SendCommand(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Gocan_SendCommand_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GocanServer).SendCommand(ctx, req.(*Command))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gocan_GetSerialPorts_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GocanServer).GetSerialPorts(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Gocan_GetSerialPorts_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GocanServer).GetSerialPorts(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gocan_GetAdapters_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GocanServer).GetAdapters(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Gocan_GetAdapters_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GocanServer).GetAdapters(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gocan_Stream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(GocanServer).Stream(&grpc.GenericServerStream[CANFrame, CANFrame]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Gocan_StreamServer = grpc.BidiStreamingServer[CANFrame, CANFrame]

// Gocan_ServiceDesc is the grpc.ServiceDesc for Gocan service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Gocan_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "Gocan",
	HandlerType: (*GocanServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SendCommand",
			Handler:    _Gocan_SendCommand_Handler,
		},
		{
			MethodName: "GetSerialPorts",
			Handler:    _Gocan_GetSerialPorts_Handler,
		},
		{
			MethodName: "GetAdapters",
			Handler:    _Gocan_GetAdapters_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Stream",
			Handler:       _Gocan_Stream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "proto/server.proto",
}
