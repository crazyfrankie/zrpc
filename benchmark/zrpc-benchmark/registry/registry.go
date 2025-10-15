package main

import (
	"flag"
	"log"

	"github.com/crazyfrankie/zrpc/registry"
)

var (
	addr      string
	keepAlive int
)

func main() {
	flag.StringVar(&addr, "addr", "127.0.0.1:8084", "service registry address")
	flag.IntVar(&keepAlive, "keepalive", 120, "service registry keepalive time in seconds")

	flag.Parse()

	tcpRegistry, err := registry.NewTcpRegistry(addr, keepAlive)
	if err != nil {
		panic(err)
	}
	log.Printf("tcp registry start at %s with keepalive %d seconds\n", addr, keepAlive)

	tcpRegistry.Serve()
}
