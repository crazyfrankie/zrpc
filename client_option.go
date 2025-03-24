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
	defaultMaxPoolSize                 = 50
	defaultHeartbeatInterval           = 30 * time.Second
	defaultHeartbeatTimeout            = 5 * time.Second
	defaultRequestTimeout              = 30 * time.Second
	defaultMaxRetries                  = 2
	defaultRetryBackoff                = 100 * time.Millisecond
	defaultMaxRetryBackoff             = 1 * time.Second
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
	requestTimeout   time.Duration
	maxRetries       int
	retryBackoff     time.Duration // Retry backoff base time
	maxRetryBackoff  time.Duration // Maximum retry backoff time
}

func defaultClientOption() *clientOption {
	return &clientOption{
		connectTimeout:        defaultConnectTimeout,
		maxSendMessageSize:    defaultClientMaxSendMessageSize,
		maxReceiveMessageSize: defaultClientMaxReceiveMessageSize,
		maxPoolSize:           defaultMaxPoolSize,
		heartbeatInterval:     defaultHeartbeatInterval,
		heartbeatTimeout:      defaultHeartbeatTimeout,
		requestTimeout:        defaultRequestTimeout,
		maxRetries:            defaultMaxRetries,
		retryBackoff:          defaultRetryBackoff,
		maxRetryBackoff:       defaultMaxRetryBackoff,
	}
}

type ClientOption func(*clientOption)

// DialWithTLSConfig establishes a TLS-based TCP connection
func DialWithTLSConfig(tls *tls.Config) ClientOption {
	return func(opt *clientOption) {
		opt.tls = tls
	}
}

// DialWithConnectTimeout sets the timeout for establishing a TCP connection
func DialWithConnectTimeout(period time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.connectTimeout = period
	}
}

// DialWithIdleTimeout sets the timeout for TCP connections to hang.
func DialWithIdleTimeout(idle time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.idleTimeout = idle
	}
}

// DialWithTCPKeepAlive sets the TCP keep-alive time.
func DialWithTCPKeepAlive(period time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.tcpKeepAlivePeriod = period
	}
}

// DialWithMaxPoolSize sets the maximum number of connection pools.
func DialWithMaxPoolSize(size int) ClientOption {
	return func(opt *clientOption) {
		opt.maxPoolSize = size
	}
}

// DialWithHeartbeatInterval sets the interval for heartbeat detection.
func DialWithHeartbeatInterval(interval time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.heartbeatInterval = interval
	}
}

// DialWithHeartbeatTimeout sets the timeout for heartbeat detection
func DialWithHeartbeatTimeout(timeout time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.heartbeatTimeout = timeout
	}
}

// DialWithRequestTimeout sets the timeout for RPC requests
func DialWithRequestTimeout(timeout time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.requestTimeout = timeout
	}
}

// DialWithMaxRetries Sets the maximum number of retries.
func DialWithMaxRetries(retries int) ClientOption {
	return func(opt *clientOption) {
		opt.maxRetries = retries
	}
}

// DialWithRetryBackoff sets the retry backoff time
func DialWithRetryBackoff(backoff time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.retryBackoff = backoff
	}
}

// DialWithMaxRetryBackoff sets the maximum retry backoff time
func DialWithMaxRetryBackoff(maxBackoff time.Duration) ClientOption {
	return func(opt *clientOption) {
		opt.maxRetryBackoff = maxBackoff
	}
}
