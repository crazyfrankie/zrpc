package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/crazyfrankie/zrpc/registry"
)

func main() {
	// 启动TCP注册中心
	addr := ":8000"
	keepaliveSec := 60 // 60秒心跳超时

	// 创建并启动注册中心
	reg, err := registry.StartTcpRegistry(addr, keepaliveSec)
	if err != nil {
		fmt.Printf("启动注册中心失败: %v\n", err)
		os.Exit(1)
	}
	defer reg.Close()

	fmt.Printf("TCP注册中心已启动于 %s\n", addr)
	fmt.Println("按Ctrl+C退出")

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭注册中心...")
}
