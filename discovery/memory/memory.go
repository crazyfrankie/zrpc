package memory

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/crazyfrankie/zrpc/discovery"
)

type MultiServerRegistry struct {
	r       *rand.Rand
	mu      sync.RWMutex
	servers []string
	idx     int
}

func NewMultiServerRegistry(servers []string) discovery.Discovery {
	rgs := &MultiServerRegistry{
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
		servers: servers,
	}
	rgs.idx = rgs.r.Intn(math.MaxInt32 - 1)

	return rgs
}

func (m *MultiServerRegistry) Get(mode discovery.SelectMode) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := len(m.servers)
	if n == 0 {
		return "", errors.New("zrpc discovery: no available servers")
	}

	switch mode {
	case discovery.RandomSelect:
		return m.servers[m.r.Intn(n)], nil
	case discovery.RoundRobinSelect:
		s := m.servers[m.idx%n] // servers could be updated, so mode n to ensure safety
		m.idx = (m.idx + 1) % n
		return s, nil
	default:
		return "", errors.New("zrpc discovery: not supported select mode")
	}
}

func (m *MultiServerRegistry) Update(servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = servers
	return nil
}

func (m *MultiServerRegistry) GetAll() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// return a copy of d.servers
	servers := make([]string, len(m.servers), len(m.servers))
	copy(servers, m.servers)
	return servers, nil
}

func (m *MultiServerRegistry) Refresh() error {
	return nil
}
