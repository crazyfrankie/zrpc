package server

import (
	"crypto/tls"
	"fmt"
	"net"
)

var makeListeners = map[string]MakeListener{
	"tcp":  tcpMakeListener("tcp"),
	"tcp4": tcpMakeListener("tcp4"),
	"tcp6": tcpMakeListener("tcp6"),
}

type MakeListener func(s *Server, address string) (net.Listener, error)

func (s *Server) makeListener(network, address string) (net.Listener, error) {
	ml, ok := makeListeners[network]
	if !ok {
		return nil, fmt.Errorf("can not make listener for %s", network)
	}

	return ml(s, address)
}

func tcpMakeListener(network string) MakeListener {
	return func(s *Server, address string) (lis net.Listener, err error) {
		if s.tlsConfig == nil {
			lis, err = net.Listen(network, address)
		} else {
			lis, err = tls.Listen(network, address, s.tlsConfig)
		}

		return
	}
}
