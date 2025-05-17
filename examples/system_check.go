package main

import (
	"fmt"
	"runtime"
	"syscall"
)

func main() {
	// 输出系统和Go运行时信息
	fmt.Printf("OS: %s\n", runtime.GOOS)
	fmt.Printf("Architecture: %s\n", runtime.GOARCH)
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("NumCPU: %d\n", runtime.NumCPU())
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))

	// 获取系统文件描述符限制
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		fmt.Printf("Error getting file descriptor limit: %v\n", err)
	} else {
		fmt.Printf("File descriptor limit: current=%d, max=%d\n", rLimit.Cur, rLimit.Max)
	}

	// 尝试增加文件描述符限制
	if runtime.GOOS != "windows" {
		newLimit := rLimit
		newLimit.Cur = 65535
		if newLimit.Cur > rLimit.Max {
			newLimit.Cur = rLimit.Max
		}

		err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &newLimit)
		if err != nil {
			fmt.Printf("Error setting file descriptor limit: %v\n", err)
			fmt.Println("You may need to run as root or modify system limits.")
			fmt.Println("Try: 'ulimit -n 65535' or add to /etc/security/limits.conf")
		} else {
			// 确认新的限制已设置
			err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
			if err == nil {
				fmt.Printf("New file descriptor limit: current=%d, max=%d\n", rLimit.Cur, rLimit.Max)
			}
		}
	}

	// 检查标准的系统限制和建议
	checkConcurrencyLimits()
}

func checkConcurrencyLimits() {
	// 建议的连接池大小
	fmt.Println("\n--- 高并发优化建议 ---")

	cpus := runtime.NumCPU()

	// 连接池大小建议
	fmt.Printf("针对不同并发级别的建议连接池大小:\n")
	fmt.Printf("- 100 并发: %d (客户端连接池), %d (服务端工作池)\n", cpus*4, cpus*6)
	fmt.Printf("- 1000 并发: %d (客户端连接池), %d (服务端工作池)\n", cpus*16, cpus*24)
	fmt.Printf("- 10000 并发: %d (客户端连接池), %d (服务端工作池)\n", cpus*32, cpus*48)

	// 系统参数调整建议
	fmt.Println("\n系统参数调整建议:")
	fmt.Println("1. 增加系统文件描述符限制: ulimit -n 65535")
	fmt.Println("2. 调整内核参数:")
	fmt.Println("   echo 'net.core.somaxconn=65535' >> /etc/sysctl.conf")
	fmt.Println("   echo 'net.ipv4.tcp_max_syn_backlog=65535' >> /etc/sysctl.conf")
	fmt.Println("   echo 'net.core.netdev_max_backlog=65535' >> /etc/sysctl.conf")
	fmt.Println("   sysctl -p")

	// 架构建议
	fmt.Println("\n架构建议:")
	fmt.Println("1. 使用多台服务器，通过负载均衡分发请求")
	fmt.Println("2. 考虑使用连接池多路复用（connection pooling multiplexing）")
	fmt.Println("3. 实现客户端限流，控制并发请求数")
	fmt.Println("4. 优化内存使用，减少不必要的对象分配")
	fmt.Println("5. 考虑使用异步处理模式或流处理")
}
