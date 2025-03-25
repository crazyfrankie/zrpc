package main

import "github.com/crazyfrankie/zrpc/registry"

func main() {
	rgs, err := registry.NewTcpRegistry(":8084", 6000)
	if err != nil {
		panic(err)
	}

	rgs.Serve()
}
