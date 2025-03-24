// Code generated by protoc-gen-go-zrpc. DO NOT EDIT.
// versrions:
// -protoc-gen-go-zrpc v0.1.0
// -protoc             v5.29.3

package bench

import (
	context "context"
	fmt "fmt"
	zrpc "github.com/crazyfrankie/zrpc"
)

const (
	HelloService_Say_FullMethodName = "bench.HelloService/Say"
)

// HelloServiceClient is the API for HelloService service.

type HelloServiceClient interface {
	Say(ctx context.Context, in *BenchmarkMessage) (*BenchmarkMessage, error)
}

type helloServiceClient struct {
	cli zrpc.ClientInterface
}

func NewHelloServiceClient(cc zrpc.ClientInterface) HelloServiceClient {
	return &helloServiceClient{cc}
}

func (c *helloServiceClient) Say(ctx context.Context, in *BenchmarkMessage) (*BenchmarkMessage, error) {
	out := new(BenchmarkMessage)
	err := c.cli.Invoke(ctx, HelloService_Say_FullMethodName, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// HelloServiceServer is the server API for HelloService service
// All implementations must embed UnimplementedHelloServiceServer
// for forward compatibility.
type HelloServiceServer interface {
	Say(context.Context, *BenchmarkMessage) (*BenchmarkMessage, error)
	mustEmbedUnimplementedHelloServiceServer()
}

// UnimplementedHelloServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedHelloServiceServer struct{}

func (UnimplementedHelloServiceServer) Say(context.Context, *BenchmarkMessage) (*BenchmarkMessage, error) {
	return nil, fmt.Errorf("method Say not implemented")
}
func (UnimplementedHelloServiceServer) mustEmbedUnimplementedHelloServiceServer() {}
func (UnimplementedHelloServiceServer) testEmbeddedByValue()                      {}

// UnsafeHelloServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to HelloServiceServer will
// result in compilation errors.
type UnsafeHelloServiceServer interface {
	mustEmbedUnimplementedHelloServiceServer()
}

func RegisterHelloServiceServer(s zrpc.ServiceRegistrar, srv HelloServiceServer) {
	// If the following call panics, it indicates UnimplementedHelloServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&HelloService_ServiceDesc, srv)
}

func _HelloService_Say_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error) (interface{}, error) {
	in := new(BenchmarkMessage)
	if err := dec(in); err != nil {
		return nil, err
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(HelloServiceServer).Say(ctx, req.(*BenchmarkMessage))
	}
	return handler(ctx, in)
}

// HelloService_ServiceDesc is the zrpc.ServiceDesc for HelloService service.
// It's only intended for direct use withzrpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var HelloService_ServiceDesc = zrpc.ServiceDesc{
	ServiceName: "bench.HelloService",
	HandlerType: (*HelloServiceServer)(nil),
	Methods: []zrpc.MethodDesc{
		{
			MethodName: "Say",
			Handler:    _HelloService_Say_Handler,
		},
	},
	Metadata: "bench.proto",
}
