package zrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/crazyfrankie/zrpc/discovery/etcd"
	"github.com/crazyfrankie/zrpc/discovery/memory"
	"sync"
	"time"

	"github.com/crazyfrankie/zrpc/discovery"
	"github.com/crazyfrankie/zrpc/metadata"
)

var (
	ErrClientConnClosing = errors.New("zrpc: the client connection is closing")
	ErrNoAvailableConn   = errors.New("zrpc: no available connection")
	ErrRequestTimeout    = errors.New("zrpc: request timeout")
	ErrResponseMismatch  = errors.New("zrpc: response sequence mismatch")
	ErrInvalidArgument   = errors.New("zrpc: invalid argument")
	ErrConnectionReset   = errors.New("zrpc: connection reset")
	ErrMaxRetryExceeded  = errors.New("zrpc: max retry exceeded")
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

	discovery discovery.Discovery
	mu        sync.RWMutex
	pools     map[string]*connPool // target -> connPool mapping

	pending  map[uint64]*Call // pending represents a request that is being processed
	sequence uint64           // sequence represents one communication, now atomic
	closing  bool             // user has called Close
	shutdown bool             // server has told us to stop

	heartbeatTicker *time.Ticker
	heartbeatDone   chan struct{}
}

// NewClient creates a new channel for the target machine,
// target can be one of:
// - "localhost:8080" - direct server address
// - "registry:///serviceName" - service name in the registry (uses the optional registryAddr from options)
// - "etcd:///serviceName" - service name in etcd (uses the etcd endpoints from options)
func NewClient(target string, opts ...ClientOption) (*Client, error) {
	client := &Client{
		opt:     defaultClientOption(),
		pools:   make(map[string]*connPool),
		pending: make(map[uint64]*Call),
	}
	for _, o := range opts {
		o(client.opt)
	}

	err := client.parserTarget(target)
	if err != nil {
		return nil, err
	}

	// initiate heartbeat detection
	if client.opt.heartbeatInterval > 0 {
		client.startHeartbeat()
	}

	return client, nil
}

func (c *Client) parserTarget(target string) error {
	// Parse the target string to determine the discovery method
	// registry:///serviceName, etcd:///serviceName, or direct server address
	if len(target) > 0 {
		switch {
		case target == "":
			return fmt.Errorf("target cannot be empty")

		case target == "registry":
			return fmt.Errorf("registry target must be in format registry:///serviceName")

		case len(target) >= 12 && target[:12] == "registry:///":
			// Registry-based service discovery: registry:///serviceName
			serviceName := target[12:]
			if serviceName == "" {
				return fmt.Errorf("service name cannot be empty in registry:/// target")
			}

			if c.opt.registryAddr == "" {
				return fmt.Errorf("registry address not specified, use WithRegistryAddress option")
			}

			// Create discovery based on registry
			c.discovery = memory.NewRegistryDiscovery(c.opt.registryAddr, serviceName)

			// Get initial servers from registry
			servers, err := c.discovery.GetAll()
			if err != nil {
				return fmt.Errorf("failed to get servers from registry: %w", err)
			}

			if len(servers) == 0 {
				return fmt.Errorf("no servers found in registry for service %s", serviceName)
			}

			// Initialize connection pools from registry
			for _, server := range servers {
				pool := newConnPool(c, server, c.opt.maxPoolSize)
				c.pools[server] = pool
			}

		case len(target) >= 8 && target[:8] == "etcd:///":
			// Etcd-based service discovery: etcd:///serviceName
			serviceName := target[8:]
			if serviceName == "" {
				return fmt.Errorf("service name cannot be empty in etcd:/// target")
			}

			if len(c.opt.etcdEndpoints) == 0 {
				return fmt.Errorf("etcd endpoints not specified, use WithEtcdEndpoints option")
			}

			// Create discovery based on etcd
			d, err := etcd.NewDiscovery(c.opt.etcdEndpoints, serviceName)
			if err != nil {
				return fmt.Errorf("failed to create etcd discovery: %w", err)
			}
			c.discovery = d

			// Get initial servers from etcd
			servers, err := c.discovery.GetAll()
			if err != nil {
				return fmt.Errorf("failed to get servers from etcd: %w", err)
			}

			if len(servers) == 0 {
				return fmt.Errorf("no servers found in etcd for service %s", serviceName)
			}

			// Initialize connection pools from etcd
			for _, server := range servers {
				pool := newConnPool(c, server, c.opt.maxPoolSize)
				c.pools[server] = pool
			}

		default:
			// Direct connection to specified address
			c.discovery = memory.NewMultiServerRegistry([]string{target})

			// Create connection pool for the target
			pool := newConnPool(c, target, c.opt.maxPoolSize)
			c.pools[target] = pool
		}
	} else {
		return fmt.Errorf("target is required")
	}

	return nil
}

func (c *Client) startHeartbeat() {
	c.heartbeatTicker = time.NewTicker(c.opt.heartbeatInterval)
	c.heartbeatDone = make(chan struct{})

	go func() {
		for {
			select {
			case <-c.heartbeatTicker.C:
				c.sendHeartbeat()
			case <-c.heartbeatDone:
				return
			}
		}
	}()
}

// sendHeartbeat create a simple heartbeat request
func (c *Client) sendHeartbeat() {
	// Get a server using random selection for heartbeat
	target, err := c.discovery.Get(discovery.RandomSelect)
	if err != nil {
		return
	}

	pool, ok := c.pools[target]
	if !ok {
		return
	}

	conn, err := pool.get()
	if err != nil {
		return
	}
	defer pool.put(conn)

	ctx, cancel := context.WithTimeout(context.Background(), c.opt.heartbeatTimeout)
	defer cancel()

	call := &Call{}
	c.sendMsg(ctx, conn, call)
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing {
		return
	}

	c.closing = true
	c.shutdown = true

	// stop Heartbeat Detection
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
		close(c.heartbeatDone)
	}

	// Close all connection pools
	for _, pool := range c.pools {
		if pool != nil {
			pool.Close()
		}
	}

	// clear all pending requests
	for _, call := range c.pending {
		call.Err = ErrClientConnClosing
		call.done()
	}
	c.pending = nil
}

func GetMeta(ctx context.Context) (metadata.MD, bool) {
	md, ok := ctx.Value(responseKey{}).(metadata.MD)
	if !ok {
		return nil, false
	}

	return md, true
}

// UpdateServers updates the list of available servers
func (c *Client) UpdateServers(servers []string) error {
	if err := c.discovery.Update(servers); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a map of existing servers
	existingServers := make(map[string]struct{})
	for _, s := range servers {
		existingServers[s] = struct{}{}
		// Create pool for new server if it doesn't exist
		if _, ok := c.pools[s]; !ok {
			c.pools[s] = newConnPool(c, s, c.opt.maxPoolSize)
		}
	}

	// Remove pools for servers that are no longer in the list
	for addr, pool := range c.pools {
		if _, ok := existingServers[addr]; !ok {
			pool.Close()
			delete(c.pools, addr)
		}
	}

	return nil
}
