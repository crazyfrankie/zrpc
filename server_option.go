package zrpc

import (
	"context"
	"crypto/tls"
	"github.com/crazyfrankie/zrpc/protocol"
	"math"
	"time"
)

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 5
	defaultServerMaxSendMessageSize    = math.MaxInt32
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
}

var defaultServerOption = &serverOption{
	readTimeout:           time.Second * 120,
	writeTimeout:          time.Second * 120,
	maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
	maxSendMessageSize:    defaultServerMaxSendMessageSize,
}

type Option func(*serverOption)

func WithReadTimeout(duration time.Duration) Option {
	return func(opt *serverOption) {
		opt.readTimeout = duration
	}
}

func WithWriteTimeout(duration time.Duration) Option {
	return func(opt *serverOption) {
		opt.writeTimeout = duration
	}
}

func WithTLSConfig(tls *tls.Config) Option {
	return func(opt *serverOption) {
		opt.tlsConfig = tls
	}
}

func WithMaxReceiveMessageSize(max int) Option {
	return func(opt *serverOption) {
		opt.maxReceiveMessageSize = max
	}
}

func WithMaxSendMessageSize(max int) Option {
	return func(opt *serverOption) {
		opt.maxSendMessageSize = max
	}
}
