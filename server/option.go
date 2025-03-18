package server

import (
	"crypto/tls"
	"time"
)

type serverOption struct {
	tlsConfig             *tls.Config
	readTimeout           time.Duration
	writeTimeout          time.Duration
	maxReceiveMessageSize int
	maxSendMessageSize    int
}

var defaultServerOption = &serverOption{}

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
