package zrpc

import (
	"crypto/tls"
	"math"
	"time"
)

const (
	defaultClientMaxReceiveMessageSize = 1024 * 1024 * 5
	defaultClientMaxSendMessageSize    = math.MaxInt32
	defaultConnectTimeout              = 20 * time.Second
)

type clientOption struct {
	tls *tls.Config
	// connectTimeout sets timeout for dialing
	connectTimeout time.Duration
	// tcpKeepAlivePeriod if it is zero we don't set keepalive
	tcpKeepAlivePeriod time.Duration
	// idleTimeout sets max idle time for underlying net.Conns
	idleTimeout           time.Duration
	maxReceiveMessageSize int
	maxSendMessageSize    int
}

func defaultClientOption() *clientOption {
	return &clientOption{
		connectTimeout:        defaultConnectTimeout,
		maxSendMessageSize:    defaultClientMaxSendMessageSize,
		maxReceiveMessageSize: defaultClientMaxReceiveMessageSize,
	}
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
