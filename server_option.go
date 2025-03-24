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
	defaultWorkerPoolSize              = 10
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

	// 工作池相关配置
	enableWorkerPool bool
	workerPoolSize   int
	taskQueueSize    int
}

var defaultServerOption = &serverOption{
	readTimeout:           time.Second * 120,
	writeTimeout:          time.Second * 120,
	maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
	maxSendMessageSize:    defaultServerMaxSendMessageSize,
	enableWorkerPool:      false,
	workerPoolSize:        defaultWorkerPoolSize,
	taskQueueSize:         defaultTaskQueueSize,
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

// 工作池相关选项
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
