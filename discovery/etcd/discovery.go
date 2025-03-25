package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"

	"github.com/crazyfrankie/zrpc/discovery"
)

// Discovery etcd-based service discovery
type Discovery struct {
	ctx    context.Context
	cancel context.CancelFunc

	client          *clientv3.Client
	serviceName     string
	servers         []string
	r               *rand.Rand
	mu              sync.RWMutex
	idx             int
	refreshInterval time.Duration
	lastRefresh     time.Time
	watchCh         clientv3.WatchChan
}

// NewDiscovery Create etcd-based service discovery
func NewDiscovery(endpoints []string, serviceName string) (discovery.Discovery, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("connect etcd failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Discovery{
		client:          cli,
		serviceName:     serviceName,
		servers:         make([]string, 0),
		r:               rand.New(rand.NewSource(time.Now().UnixNano())),
		refreshInterval: time.Minute,
		ctx:             ctx,
		cancel:          cancel,
	}

	if err := d.Refresh(); err != nil {
		cli.Close()
		cancel()
		return nil, err
	}

	go d.watch()

	return d, nil
}

// watch for service changes
func (d *Discovery) watch() {
	prefix := d.getKeyPrefix()
	d.watchCh = d.client.Watch(d.ctx, prefix, clientv3.WithPrefix())

	for wresp := range d.watchCh {
		for _, ev := range wresp.Events {
			switch ev.Type {
			case clientv3.EventTypePut:
				//TODO
			case clientv3.EventTypeDelete:
				//TODO
			}
		}

		d.Refresh()
	}
}

// Refresh the list of services from etcd.
func (d *Discovery) Refresh() error {
	prefix := d.getKeyPrefix()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	resp, err := d.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("从etcd获取服务列表失败: %v", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.servers = make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var info endpoints.Endpoint
		if err := json.Unmarshal(kv.Value, &info); err != nil {
			continue
		}
		d.servers = append(d.servers, info.Addr)
	}

	d.lastRefresh = time.Now()
	return nil
}

// Update updates the list of services
func (d *Discovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.servers = servers
	return nil
}

// Get a service address based on a load balancing policy
func (d *Discovery) Get(mode discovery.SelectMode) (string, error) {
	d.mu.RLock()

	if len(d.servers) == 0 || time.Since(d.lastRefresh) > d.refreshInterval {
		d.mu.RUnlock()
		if err := d.Refresh(); err != nil {
			return "", err
		}
		d.mu.RLock()
	}

	n := len(d.servers)
	if n == 0 {
		d.mu.RUnlock()
		return "", discovery.ErrNoServers
	}

	var addr string
	switch mode {
	case discovery.RandomSelect:
		addr = d.servers[d.r.Intn(n)]
	case discovery.RoundRobinSelect:
		addr = d.servers[d.idx%n]
		d.idx = (d.idx + 1) % n
	default:
		d.mu.RUnlock()
		return "", discovery.ErrUnsupportedMode
	}

	d.mu.RUnlock()
	return addr, nil
}

// GetAll Get all service addresses
func (d *Discovery) GetAll() ([]string, error) {
	d.mu.RLock()

	// Check if a refresh is needed
	if len(d.servers) == 0 || time.Since(d.lastRefresh) > d.refreshInterval {
		d.mu.RUnlock()
		if err := d.Refresh(); err != nil {
			return nil, err
		}
		d.mu.RLock()
	}

	servers := make([]string, len(d.servers))
	copy(servers, d.servers)
	d.mu.RUnlock()

	return servers, nil
}

// Close Turns off service discovery
func (d *Discovery) Close() error {
	d.cancel()
	return d.client.Close()
}

// getKeyPrefix Get the key prefix of the service in etcd
func (d *Discovery) getKeyPrefix() string {
	return fmt.Sprintf("service/%s/", d.serviceName)
}

// getKey Get the key for the service in etcd.
func (d *Discovery) getKey(addr string) string {
	return fmt.Sprintf("service/%s/%s", d.serviceName, addr)
}
