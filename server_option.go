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
)

type serverOption struct {
	tlsConfig             *tls.Config
	readTimeout           time.Duration
	writeTimeout          time.Duration
	maxReceiveMessageSize int
	maxSendMessageSize    int
	// AuthFunc can be used to auth.
	AuthFunc        func(ctx context.Context, req *protocol.Message, token string) error
	ServerErrorFunc func(res *protocol.Message, err error) string

	enableWorkerPool bool
	workerPoolSize   int
	taskQueueSize    int
	enableDebug      bool // 启用调试模式，会输出更多日志
}

var defaultServerOption = &serverOption{
	readTimeout:           time.Second * 120,
	writeTimeout:          time.Second * 120,
	maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
	maxSendMessageSize:    defaultServerMaxSendMessageSize,
	enableWorkerPool:      false,
	workerPoolSize:        defaultWorkerPoolSize,
	taskQueueSize:         defaultTaskQueueSize,
	enableDebug:           false,
}

type ServerOption func(*serverOption)

func WithReadTimeout(duration time.Duration) ServerOption {
	return func(opt *serverOption) {
		opt.readTimeout = duration
	}
}

func WithWriteTimeout(duration time.Duration) ServerOption {
	return func(opt *serverOption) {
		opt.writeTimeout = duration
	}
}

func WithTLSConfig(tls *tls.Config) ServerOption {
	return func(opt *serverOption) {
		opt.tlsConfig = tls
	}
}

func WithMaxReceiveMessageSize(max int) ServerOption {
	return func(opt *serverOption) {
		opt.maxReceiveMessageSize = max
	}
}

func WithMaxSendMessageSize(max int) ServerOption {
	return func(opt *serverOption) {
		opt.maxSendMessageSize = max
	}
}

func WithWorkerPool(size int) ServerOption {
	return func(opt *serverOption) {
		opt.enableWorkerPool = true
		if size > 0 {
			opt.workerPoolSize = size
		}
	}
}

func WithTaskQueueSize(size int) ServerOption {
	return func(opt *serverOption) {
		if size > 0 {
			opt.taskQueueSize = size
		}
	}
}

// WithDebug 启用调试模式，会输出更多日志
func WithDebug() ServerOption {
	return func(opt *serverOption) {
		opt.enableDebug = true
	}
}
