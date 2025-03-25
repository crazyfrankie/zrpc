package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/crazyfrankie/zrpc"
	"github.com/crazyfrankie/zrpc/registry"
)

// 服务接口定义
type IHelloService interface {
	SayHello(ctx context.Context, request *HelloRequest, response *HelloResponse) error
}

// 示例服务
type HelloService struct{}

func (h *HelloService) SayHello(ctx context.Context, request *HelloRequest, response *HelloResponse) error {
	response.Message = "Hello, " + request.Name
	return nil
}

// 请求和响应结构体
type HelloRequest struct {
	Name string
}

type HelloResponse struct {
	Message string
}

// 构建服务描述
var _HelloServiceDesc = &zrpc.ServiceDesc{
	ServiceName: "HelloService",
	HandlerType: (*IHelloService)(nil),
	Methods: []zrpc.MethodDesc{
		{
			MethodName: "SayHello",
			Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error) (interface{}, error) {
				in := new(HelloRequest)
				if err := dec(in); err != nil {
					return nil, err
				}
				out := new(HelloResponse)
				if err := srv.(IHelloService).SayHello(ctx, in, out); err != nil {
					return nil, err
				}
				return out, nil
			},
		},
	},
}

func main() {
	// 1. 启动RPC服务
	serverAddr := "localhost:8090"
	serviceName := "HelloService"

	// 创建服务器
	server := zrpc.NewServer()

	// 注册服务
	server.RegisterService(_HelloServiceDesc, new(HelloService))

	// 在后台启动服务
	go func() {
		fmt.Printf("RPC服务已启动在 %s\n", serverAddr)
		err := server.Serve("tcp", serverAddr)
		if err != nil {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 2. 注册到TCP注册中心
	// 创建TCP客户端连接注册中心
	registryAddr := "localhost:8000"
	client := registry.NewTcpClient(registryAddr)

	// 注册服务
	metadata := map[string]string{
		"version": "1.0",
		"weight":  "10",
	}

	err := client.Register(serviceName, serverAddr, metadata)
	if err != nil {
		log.Fatalf("注册服务到注册中心失败: %v", err)
	}
	defer client.Unregister()

	fmt.Printf("服务 %s 已注册到注册中心 %s\n", serviceName, registryAddr)
	fmt.Println("按Ctrl+C退出")

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭服务...")

	// 优雅关闭
	server.GracefulStop()
	time.Sleep(time.Second) // 给服务器时间完成关闭
}
