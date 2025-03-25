package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/crazyfrankie/zrpc/registry"
)

func main() {
	// 创建TCP客户端
	client := registry.NewTcpClient("localhost:8000")

	// 注册服务
	serviceName := "hello"
	serviceAddr := "localhost:8001" // 假设服务运行在这个地址
	metadata := map[string]string{
		"version": "1.0",
		"weight":  "10",
	}

	err := client.Register(serviceName, serviceAddr, metadata)
	if err != nil {
		fmt.Printf("注册服务失败: %v\n", err)
		os.Exit(1)
	}
	defer client.Unregister()

	fmt.Printf("服务 %s 已注册到注册中心，地址: %s\n", serviceName, serviceAddr)
	fmt.Println("按Ctrl+C退出")

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在注销服务...")
}
