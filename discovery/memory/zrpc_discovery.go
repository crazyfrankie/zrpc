package memory

import (
	"math/rand"
	"sync"
	"time"

	"github.com/crazyfrankie/zrpc/discovery"
	"github.com/crazyfrankie/zrpc/registry"
)

// RegistryDiscovery Registry-based service discovery
type RegistryDiscovery struct {
	registryAddr    string
	serviceName     string
	tcpClient       *registry.TcpClient
	servers         []string
	r               *rand.Rand
	mu              sync.RWMutex
	idx             int
	refreshInterval time.Duration
	lastRefresh     time.Time
}

// NewRegistryDiscovery Create registry-based service discovery
func NewRegistryDiscovery(registryAddr, serviceName string) discovery.Discovery {
	client := registry.NewTcpClient(registryAddr)

	d := &RegistryDiscovery{
		registryAddr:    registryAddr,
		serviceName:     serviceName,
		tcpClient:       client,
		servers:         make([]string, 0),
		r:               rand.New(rand.NewSource(time.Now().UnixNano())),
		refreshInterval: time.Minute,
	}

	d.Refresh()

	return d
}

// Refresh the list of services from the registry
func (d *RegistryDiscovery) Refresh() error {
	servers, err := d.tcpClient.GetService(d.serviceName)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.servers = servers
	d.lastRefresh = time.Now()
	return nil
}

// Update Updates the list of services
func (d *RegistryDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.servers = servers
	return nil
}

// Get returns a service address based on a load balancing policy
func (d *RegistryDiscovery) Get(mode discovery.SelectMode) (string, error) {
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

// GetAll returns all service addresses
func (d *RegistryDiscovery) GetAll() ([]string, error) {
	d.mu.RLock()

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
