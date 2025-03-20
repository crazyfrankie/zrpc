package zrpc

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"

	"go.uber.org/zap"
)

var (
	ErrClientConnClosing = errors.New("zrpc: the client connection is closing")
)

const (
	// ReaderBufferSize is used for bufio reader.
	ReaderBufferSize = 16 * 1024
)

type ClientInterface interface {
	Invoke(ctx context.Context, method string, args any, reply any) error
}

// Assert *ClientConn implements ClientConnInterface.
var _ ClientInterface = (*Client)(nil)

// Client represents a virtual connection to a conceptual endpoint, to
// perform RPCs.
//
// A Client is free to have zero or more actual connections to the endpoint
// based on configuration, load, etc. It is also free to determine which actual
// endpoints to use and may change it every RPC, permitting client-side load
// balancing.
//
// A Client encapsulates a range of functionality including name
// resolution, TCP connection establishment (with retries and backoff) and TLS
// handshakes. It also handles errors on established connections by
// re-resolving the name and reconnecting.
type Client struct {
	opt *clientOption

	target   string
	mu       sync.RWMutex
	conns    map[*connWrapper]struct{}
	closing  bool // user has called Close
	shutdown bool // server has told us to stop
}

// NewClient creates a new channel for the target machine,
func NewClient(target string, opts ...ClientOption) (*Client, error) {
	client := &Client{
		opt:    defaultClientOption(),
		target: target,
		conns:  make(map[*connWrapper]struct{}),
	}
	for _, o := range opts {
		o(client.opt)
	}

	return client, nil
}

// newConnWrapper returns a
func (c *Client) newConnWrapper(target string) (*connWrapper, error) {
	if c.conns == nil {
		return nil, ErrClientConnClosing
	}

	cw := &connWrapper{
		client:   c,
		addr:     target,
		isClosed: false,
	}

	var conn net.Conn
	var err error
	var tlsConn *tls.Conn

	if c.opt.tls != nil {
		dialer := &net.Dialer{
			Timeout: c.opt.connectTimeout,
		}
		tlsConn, err = tls.DialWithDialer(dialer, "tcp", target, c.opt.tls)
		conn = net.Conn(tlsConn)
	} else {
		conn, err = net.DialTimeout("tcp", target, c.opt.connectTimeout)
	}

	if err != nil {
		zap.L().Warn("failed to dial server: ", zap.Error(err))
		return nil, err
	}

	cw.conn = conn
	cw.reader = bufio.NewReaderSize(conn, ReaderBufferSize)

	c.conns[cw] = struct{}{}

	return cw, nil
}

type connWrapper struct {
	client   *Client
	conn     net.Conn
	mu       sync.Mutex
	reader   *bufio.Reader
	addr     string
	isClosed bool
}
