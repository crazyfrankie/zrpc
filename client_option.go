package zrpc

import (
	"crypto/tls"
	"math"
	"time"

	"github.com/crazyfrankie/zrpc/discovery"
)

const (
	defaultClientMaxReceiveMessageSize = 1024 * 1024 * 5
	defaultClientMaxSendMessageSize    = math.MaxInt32
	defaultConnectTimeout              = 20 * time.Second
	defaultMaxPoolSize                 = 50
	// Application layer heartbeat defaults to 30 seconds, used to check the health of the service
	// If the heartbeat fails, the connection is closed and re-established.
	defaultHeartbeatInterval = 30 * time.Second
	// Heartbeat timeout of 5 seconds
	defaultHeartbeatTimeout = 5 * time.Second
	// TCP keep-alive defaults to 15 seconds, used to detect network connection state
	// If 0, TCP keep-alive is not enabled and relies solely on the application layer heartbeat
	defaultTCPKeepAlivePeriod = 15 * time.Second
	defaultRequestTimeout     = 30 * time.Second
	defaultMaxRetries         = 2
	defaultRetryBackoff       = 100 * time.Millisecond
	defaultMaxRetryBackoff    = 1 * time.Second
)

type clientOption struct {
	tls *tls.Config
	// connectTimeout sets timeout for dialing
	connectTimeout time.Duration
	// tcpKeepAlivePeriod is used to detect the state of the network connection
	// If 0, TCP keep-alive is not enabled and relies solely on the application layer heartbeat
	// Recommended setting is 15-30 seconds for quick detection of network problems.
	tcpKeepAlivePeriod time.Duration
	// idleTimeout sets max idle time for underlying net.Conns
	idleTimeout           time.Duration
	maxPoolSize           int
	maxReceiveMessageSize int
	maxSendMessageSize    int
	// heartbeatInterval is used to check the health of the service
	// If 0, the application layer heartbeat is not enabled.
	// Recommended to be set to 30-60 seconds to check if the service is running properly
	// If the heartbeat fails, the connection will be closed and re-established.
	heartbeatInterval time.Duration
	// heartbeatTimeout sets timeout for heartbeat request
	heartbeatTimeout time.Duration
	requestTimeout   time.Duration
	maxRetries       int
	retryBackoff     time.Duration // Retry backoff base time
	maxRetryBackoff  time.Duration // Maximum retry backoff time
	balancerMode     discovery.SelectMode
	registryAddr     string   // TCP registry address
	etcdEndpoints    []string // Etcd endpoints
}

func defaultClientOption() *clientOption {
	return &clientOption{
		connectTimeout:        defaultConnectTimeout,
		maxSendMessageSize:    defaultClientMaxSendMessageSize,
		maxReceiveMessageSize: defaultClientMaxReceiveMessageSize,
		maxPoolSize:           defaultMaxPoolSize,
		heartbeatInterval:     defaultHeartbeatInterval,
		heartbeatTimeout:      defaultHeartbeatTimeout,
		tcpKeepAlivePeriod:    defaultTCPKeepAlivePeriod,
		requestTimeout:        defaultRequestTimeout,
		maxRetries:            defaultMaxRetries,
		retryBackoff:          defaultRetryBackoff,
		maxRetryBackoff:       defaultMaxRetryBackoff,
		balancerMode:          discovery.RoundRobinSelect,
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

// DialWithBalancer sets balancer select mode
func DialWithBalancer(mode discovery.SelectMode) ClientOption {
	return func(opt *clientOption) {
		opt.balancerMode = mode
	}
}

// DialWithRegistryAddress setting the Registration Center Address
func DialWithRegistryAddress(addr string) ClientOption {
	return func(opt *clientOption) {
		opt.registryAddr = addr
	}
}

// DialWithEtcdEndpoints setting up an etcd endpoint
func DialWithEtcdEndpoints(endpoints []string) ClientOption {
	return func(opt *clientOption) {
		opt.etcdEndpoints = endpoints
	}
}
