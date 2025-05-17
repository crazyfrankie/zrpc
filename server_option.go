package zrpc

import (
	"context"
	"crypto/tls"
	"math"
	"time"

	"github.com/crazyfrankie/zrpc/protocol"
)

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 5
	defaultServerMaxSendMessageSize    = math.MaxInt32
	defaultWorkerPoolSize              = 20
	defaultTaskQueueSize               = 10000
	defaultMinWorkerPoolSize           = 5
	defaultMaxWorkerPoolSize           = 100
)

type serverOption struct {
	srvMiddleware         ServerMiddleware
	chainMiddlewares      []ServerMiddleware
	tlsConfig             *tls.Config
	readTimeout           time.Duration
	writeTimeout          time.Duration
	maxReceiveMessageSize int
	maxSendMessageSize    int
	// AuthFunc can be used to auth.
	AuthFunc        func(ctx context.Context, req *protocol.Message, token string) error
	ServerErrorFunc func(res *protocol.Message, err error) string

	enableWorkerPool  bool
	workerPoolSize    int
	minWorkerPoolSize int
	maxWorkerPoolSize int
	taskQueueSize     int
	adjustInterval    time.Duration // Interval for worker pool size adjustment
	enableDebug       bool
}

var defaultServerOption = &serverOption{
	readTimeout:           time.Second * 120,
	writeTimeout:          time.Second * 120,
	maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
	maxSendMessageSize:    defaultServerMaxSendMessageSize,
	enableWorkerPool:      false,
	workerPoolSize:        defaultWorkerPoolSize,
	minWorkerPoolSize:     defaultMinWorkerPoolSize,
	maxWorkerPoolSize:     defaultMaxWorkerPoolSize,
	taskQueueSize:         defaultTaskQueueSize,
	adjustInterval:        time.Second * 5, // Default to 5 seconds for worker pool adjustment
	enableDebug:           false,
}

type ServerOption func(*serverOption)

// WithMiddleware receives incoming middleware, adds it to chainMiddlewares as an interceptor,
// and calls it before the business logic does.
func WithMiddleware(mw ServerMiddleware) ServerOption {
	return func(opt *serverOption) {
		if opt.srvMiddleware != nil {
			panic("The server middleware was already set and may not be reset.")
		}
		opt.srvMiddleware = mw
	}
}

// WithChainMiddleware works like WithMiddleware
// in that it takes multiple middleware and adds them to chainMiddlewares at once.
func WithChainMiddleware(mws []ServerMiddleware) ServerOption {
	return func(opt *serverOption) {
		opt.chainMiddlewares = append(opt.chainMiddlewares, mws...)
	}
}

// WithReadTimeout sets the timeout for a read request.
func WithReadTimeout(duration time.Duration) ServerOption {
	return func(opt *serverOption) {
		opt.readTimeout = duration
	}
}

// WithWriteTimeout sets the timeout for writing responses
func WithWriteTimeout(duration time.Duration) ServerOption {
	return func(opt *serverOption) {
		opt.writeTimeout = duration
	}
}

// WithTLSConfig sets the TLS configuration for whether to make a secure connection when connecting;
// if it is empty, a secure connection is not used
func WithTLSConfig(tls *tls.Config) ServerOption {
	return func(opt *serverOption) {
		opt.tlsConfig = tls
	}
}

// WithMaxReceiveMessageSize sets the maximum size of request messages the server can accept,
// otherwise the default value is used.
func WithMaxReceiveMessageSize(max int) ServerOption {
	return func(opt *serverOption) {
		opt.maxReceiveMessageSize = max
	}
}

// WithMaxSendMessageSize sets the size of the maximum message body that the server can send,
// otherwise it is the default value.
func WithMaxSendMessageSize(max int) ServerOption {
	return func(opt *serverOption) {
		opt.maxSendMessageSize = max
	}
}

// WithWorkerPool sets the size of the server-side worker pool.
// Because it's a dynamic policy, the minimum value is one-quarter of the value passed in,
// and the maximum value is twice the value passed in, initially.
func WithWorkerPool(size int) ServerOption {
	return func(opt *serverOption) {
		opt.enableWorkerPool = true
		if size > 0 {
			opt.workerPoolSize = size
			opt.minWorkerPoolSize = size / 4
			opt.maxWorkerPoolSize = size * 2
		}
	}
}

// WithTaskQueueSize sets the size of the task pool.
func WithTaskQueueSize(size int) ServerOption {
	return func(opt *serverOption) {
		if size > 0 {
			opt.taskQueueSize = size
		}
	}
}

// WithWorkerPoolAdjustInterval sets the interval for adjusting worker pool size.
func WithWorkerPoolAdjustInterval(interval time.Duration) ServerOption {
	return func(opt *serverOption) {
		if interval > 0 {
			opt.adjustInterval = interval
		}
	}
}

// WithWorkerPoolSize sets the min and max size of the server-side worker pool.
func WithWorkerPoolSize(min, max int) ServerOption {
	return func(opt *serverOption) {
		opt.enableWorkerPool = true
		if min > 0 && max >= min {
			opt.minWorkerPoolSize = min
			opt.maxWorkerPoolSize = max
			opt.workerPoolSize = (min + max) / 2
		}
	}
}
