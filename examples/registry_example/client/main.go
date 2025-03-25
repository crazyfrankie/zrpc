package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/crazyfrankie/zrpc"
)

// 请求和响应结构体
type HelloRequest struct {
	Name string
}

type HelloResponse struct {
	Message string
}

func main() {
	// 示例1: 直接连接到服务器地址
	fmt.Println("===== 直接连接示例 =====")
	directClient, err := zrpc.NewClient("localhost:8090")
	if err != nil {
		log.Fatalf("创建直接连接客户端失败: %v", err)
	}
	defer directClient.Close()

	callService(directClient, "直接连接")

	// 示例2: 使用TCP注册中心发现服务
	fmt.Println("\n===== 注册中心服务发现示例 =====")
	registryClient, err := zrpc.NewClient(
		"registry:///HelloService",
		zrpc.WithRegistryAddress("localhost:8000"),
	)
	if err != nil {
		log.Fatalf("创建注册中心客户端失败: %v", err)
	}
	defer registryClient.Close()

	callService(registryClient, "注册中心")

	// 示例3: 如果有etcd环境，可以使用etcd发现服务
	// 注意：这需要启动etcd并注册服务
	/*
		etcdClient, err := zrpc.NewClient(
			"etcd:///HelloService",
			zrpc.WithEtcdEndpoints([]string{"localhost:2379"}),
		)
		if err != nil {
			log.Printf("创建etcd客户端失败: %v", err)
		} else {
			defer etcdClient.Close()
			callService(etcdClient, "etcd")
		}
	*/
}

// 调用服务
func callService(client *zrpc.Client, clientType string) {
	request := &HelloRequest{Name: "World"}
	response := &HelloResponse{}

	// 创建上下文，设置超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 调用服务
	err := client.Invoke(ctx, "HelloService.SayHello", request, response)
	if err != nil {
		log.Printf("%s 调用服务失败: %v", clientType, err)
		return
	}

	fmt.Printf("%s 调用结果: %s\n", clientType, response.Message)

	// 多次调用，展示负载均衡效果
	for i := 0; i < 3; i++ {
		request.Name = fmt.Sprintf("User%d", i+1)
		err = client.Invoke(ctx, "HelloService.SayHello", request, response)
		if err != nil {
			log.Printf("%s 第%d次调用失败: %v", clientType, i+1, err)
			continue
		}
		fmt.Printf("%s 第%d次调用结果: %s\n", clientType, i+1, response.Message)
		time.Sleep(100 * time.Millisecond)
	}
}
