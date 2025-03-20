package zrpc

import (
	"crypto/tls"
	"time"
)

type clientOption struct {
	tls *tls.Config
	// connectTimeout sets timeout for dialing
	connectTimeout time.Duration
	// tcpKeepAlivePeriod if it is zero we don't set keepalive
	tcpKeepAlivePeriod time.Duration
	// idleTimeout sets max idle time for underlying net.Conns
	idleTimeout time.Duration
}

func defaultClientOption() *clientOption {
	return &clientOption{}
}

type ClientOption func(*clientOption)

func DialWithTLSConfig(tls *tls.Config) ClientOption {
	return func(opt *clientOption) {
		opt.tls = tls
	}
}

func DialWithConnectTimeout(period time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.connectTimeout = period
	}
}

func DialWithIdleTimeout(idle time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.idleTimeout = idle
	}
}

func DialWithTCPKeepAlive(period time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.tcpKeepAlivePeriod = period
	}
}
