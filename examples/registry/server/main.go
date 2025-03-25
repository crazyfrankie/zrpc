package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/crazyfrankie/zrpc/registry"
)

func main() {
	// 启动注册中心
	tcpRegistry, err := registry.NewTcpRegistry(":8000", 60)
	if err != nil {
		fmt.Printf("启动注册中心失败: %v\n", err)
		os.Exit(1)
	}
	defer tcpRegistry.Close()

	fmt.Println("TCP注册中心已启动于 :8000")
	fmt.Println("按Ctrl+C退出")

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭注册中心...")
}
