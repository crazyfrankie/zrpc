package zrpc

import (
	"context"
	"errors"
	"github.com/crazyfrankie/zrpc/metadata"
	"math"
	"sync"
	"time"
)

var (
	ErrClientConnClosing = errors.New("zrpc: the client connection is closing")
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

	target string
	mu     sync.RWMutex
	conns  map[*clientConn]struct{}

	pending  map[uint64]*Call // pending represents a request that is being processed
	sequence uint64           // sequence represents one communication
	closing  bool             // user has called Close
	shutdown bool             // server has told us to stop
}

// NewClient creates a new channel for the target machine,
func NewClient(target string, opts ...ClientOption) (*Client, error) {
	client := &Client{
		opt:     defaultClientOption(),
		target:  target,
		conns:   make(map[*clientConn]struct{}),
		pending: make(map[uint64]*Call),
	}
	for _, o := range opts {
		o(client.opt)
	}

	return client, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing {
		return
	}

	c.closing = true
	for conn := range c.conns {
		conn.Close()
	}
	c.conns = nil
}

func (c *Client) getConn() (*clientConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing || c.shutdown {
		return nil, errors.New("client is shutting down")
	}

	for conn := range c.conns {
		if !conn.inUse {
			conn.inUse = true
			conn.lastUsed = time.Now()
			return conn, nil
		}
	}

	newConn, err := newClientConn(c, c.target)
	if err != nil {
		return nil, err
	}
	newConn.inUse = true
	c.conns[newConn] = struct{}{}
	return newConn, nil
}

func (c *Client) removeConn(conn *clientConn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn.inUse = false
	conn.lastUsed = time.Now()
}

func (c *Client) nextSeq(call *Call) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence = (c.sequence + 1) & math.MaxUint64
	c.pending[c.sequence] = call
}

func GetMeta(ctx context.Context) (metadata.MD, bool) {
	md, ok := ctx.Value(responseKey{}).(metadata.MD)
	if !ok {
		return nil, false
	}

	return md, true
}
