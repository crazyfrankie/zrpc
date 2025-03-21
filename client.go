package zrpc

import (
	"context"
	"errors"
	"github.com/crazyfrankie/zrpc/metadata"
	"sync"
)

var (
	ErrClientConnClosing = errors.New("zrpc: the client connection is closing")
	ErrNoAvailableConn   = errors.New("zrpc: no available connection")
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
	pool   *connPool

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
		pending: make(map[uint64]*Call),
	}
	for _, o := range opts {
		o(client.opt)
	}

	client.pool = newConnPool(client, target, client.opt.maxPoolSize)
	return client, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing {
		return
	}

	c.closing = true
	c.shutdown = true

	if c.pool != nil {
		c.pool.mu.Lock()
		for _, conn := range c.pool.conns {
			conn.Close()
		}
		c.pool.conns = nil
		c.pool.mu.Unlock()
	}
}

func GetMeta(ctx context.Context) (metadata.MD, bool) {
	md, ok := ctx.Value(responseKey{}).(metadata.MD)
	if !ok {
		return nil, false
	}

	return md, true
}
