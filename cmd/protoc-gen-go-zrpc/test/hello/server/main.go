package main

import (
	"context"
	"fmt"
	"log"

	"github.com/crazyfrankie/zrpc"

	rpcgen "github.com/crazyfrankie/zrpc/cmd/protoc-gen-go-zrpc/test/hello/rpc_gen"
)

func main() {
	server := zrpc.NewServer()

	rpcgen.RegisterHelloServiceServer(server, &HelloService{})
	err := server.Serve("tcp", ":8082")
	if err != nil {
		log.Println(err)
	}
}

type HelloService struct {
	rpcgen.UnimplementedHelloServiceServer
}

func (s *HelloService) HelloWorld(ctx context.Context, req *rpcgen.HelloRequest) (*rpcgen.HelloResponse, error) {
	fmt.Println(req.GetMsg())
	return &rpcgen.HelloResponse{Msg: req.GetMsg()}, nil
}
