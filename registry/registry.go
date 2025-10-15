package registry

import (
	"fmt"
	"sync"
	"time"
)

type Registry struct {
	mu      sync.RWMutex
	servers map[string]map[string]*ServiceInfo // serviceName -> addr -> serviceInfo
	timeout time.Duration
}

type ServiceInfo struct {
	Name      string            // 服务名称
	Addr      string            // 服务地址 (host:port)
	Metadata  map[string]string // 服务元数据
	StartTime time.Time         // 服务启动时间
	LastPing  time.Time         // 最近一次心跳时间
}

// NewRegistry creates a new registry, timeout is the service timeout
func NewRegistry(timeout time.Duration) *Registry {
	r := &Registry{
		servers: make(map[string]map[string]*ServiceInfo),
		timeout: timeout,
	}

	go r.cleanExpiredServers()

	return r
}

func (r *Registry) Register(serviceName, addr string, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	services, ok := r.servers[serviceName]
	if !ok {
		services = make(map[string]*ServiceInfo)
		r.servers[serviceName] = services
	}

	services[addr] = &ServiceInfo{
		Name:      serviceName,
		Addr:      addr,
		Metadata:  metadata,
		StartTime: time.Now(),
		LastPing:  time.Now(),
	}

	return nil
}

func (r *Registry) Unregister(serviceName, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	services, ok := r.servers[serviceName]
	if !ok {
		return fmt.Errorf("service %s not found", serviceName)
	}

	if _, ok := services[addr]; !ok {
		return fmt.Errorf("addr %s not found for service %s", addr, serviceName)
	}

	delete(services, addr)

	if len(services) == 0 {
		delete(r.servers, serviceName)
	}

	return nil
}

func (r *Registry) Heartbeat(serviceName, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	services, ok := r.servers[serviceName]
	if !ok {
		return fmt.Errorf("service %s not found", serviceName)
	}

	service, ok := services[addr]
	if !ok {
		return fmt.Errorf("addr %s not found for service %s", addr, serviceName)
	}

	service.LastPing = time.Now()
	return nil
}

func (r *Registry) IsExpired(serviceName, addr string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services, ok := r.servers[serviceName]
	if !ok {
		return true
	}

	service, ok := services[addr]
	if !ok {
		return true
	}

	return time.Since(service.LastPing) > r.timeout
}

func (r *Registry) GetService(serviceName string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services, ok := r.servers[serviceName]
	if !ok || len(services) == 0 {
		return nil, fmt.Errorf("no available services for %s", serviceName)
	}

	var addrs []string
	for addr := range services {
		addrs = append(addrs, addr)
	}

	return addrs, nil
}

func (r *Registry) GetAllServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var services []string
	for svc := range r.servers {
		services = append(services, svc)
	}

	return services
}

func (r *Registry) cleanExpiredServers() {
	ticker := time.NewTicker(r.timeout / 2)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		now := time.Now()

		for svcName, services := range r.servers {
			for addr, svc := range services {
				if now.Sub(svc.LastPing) > r.timeout {
					delete(services, addr)
					fmt.Printf("service expired and was removed: %s - %s\n", svcName, addr)
				}
			}

			if len(services) == 0 {
				delete(r.servers, svcName)
			}
		}
		r.mu.Unlock()
	}
}
