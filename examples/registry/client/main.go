package main

import (
	"fmt"
	"github.com/crazyfrankie/zrpc/discovery/memory"
	"time"

	"github.com/crazyfrankie/zrpc/discovery"
	"github.com/crazyfrankie/zrpc/registry"
)

func main() {
	// 方法1：直接使用TcpClient获取服务
	tcpClient := registry.NewTcpClient("localhost:8000")

	fmt.Println("=== 直接使用TcpClient获取服务 ===")
	addrs, err := tcpClient.GetService("hello")
	if err != nil {
		fmt.Printf("获取服务失败: %v\n", err)
	} else {
		fmt.Println("找到服务地址:")
		for _, addr := range addrs {
			fmt.Printf("  - %s\n", addr)
		}
	}

	// 方法2：使用RegistryDiscovery
	fmt.Println("\n=== 使用RegistryDiscovery进行服务发现 ===")
	d := internal.NewRegistryDiscovery("localhost:8000", "hello")

	// 随机获取一个服务地址
	addr, err := d.Get(discovery.RandomSelect)
	if err != nil {
		fmt.Printf("获取服务失败: %v\n", err)
	} else {
		fmt.Printf("随机选择的服务地址: %s\n", addr)
	}

	// 轮询获取服务地址（演示多次调用）
	fmt.Println("\n轮询获取服务地址:")
	for i := 0; i < 5; i++ {
		addr, err := d.Get(discovery.RoundRobinSelect)
		if err != nil {
			fmt.Printf("  获取服务失败: %v\n", err)
		} else {
			fmt.Printf("  第%d次获取: %s\n", i+1, addr)
		}
		time.Sleep(time.Millisecond * 100)
	}

	// 获取所有服务地址
	servers, err := d.GetAll()
	if err != nil {
		fmt.Printf("\n获取所有服务失败: %v\n", err)
	} else {
		fmt.Println("\n所有可用服务地址:")
		for i, server := range servers {
			fmt.Printf("  %d. %s\n", i+1, server)
		}
	}
}
