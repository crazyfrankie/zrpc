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
	defaultMaxPoolSize                 = 30
	defaultHeartbeatInterval           = 30 * time.Second
	defaultHeartbeatTimeout            = 5 * time.Second
)

type clientOption struct {
	tls *tls.Config
	// connectTimeout sets timeout for dialing
	connectTimeout time.Duration
	// tcpKeepAlivePeriod if it is zero we don't set keepalive
	tcpKeepAlivePeriod time.Duration
	// idleTimeout sets max idle time for underlying net.Conns
	idleTimeout           time.Duration
	maxPoolSize           int
	maxReceiveMessageSize int
	maxSendMessageSize    int
	// heartbeatInterval sets interval for sending heartbeat
	heartbeatInterval time.Duration
	// heartbeatTimeout sets timeout for heartbeat request
	heartbeatTimeout time.Duration
}

func defaultClientOption() *clientOption {
	return &clientOption{
		connectTimeout:        defaultConnectTimeout,
		maxSendMessageSize:    defaultClientMaxSendMessageSize,
		maxReceiveMessageSize: defaultClientMaxReceiveMessageSize,
		maxPoolSize:           defaultMaxPoolSize,
		heartbeatInterval:     defaultHeartbeatInterval,
		heartbeatTimeout:      defaultHeartbeatTimeout,
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

func DialWithMaxPoolSize(size int) ClientOption {
	return func(opt *clientOption) {
		opt.maxPoolSize = size
	}
}

func DialWithHeartbeatInterval(interval time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.heartbeatInterval = interval
	}
}

func DialWithHeartbeatTimeout(timeout time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.heartbeatTimeout = timeout
	}
}
