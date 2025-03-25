package etcd

import (
	"context"
	"fmt"
	"log"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
)

// Registry etcd service registration
type Registry struct {
	ctx    context.Context
	cancel context.CancelFunc

	client  *clientv3.Client
	em      endpoints.Manager
	leaseID clientv3.LeaseID

	key string
	val endpoints.Endpoint
	ttl int64
}

// RegisterService Registers the service to etcd.
func RegisterService(addr []string, serviceName, serviceAddr string, metadata map[string]string, ttl int64) (*Registry, error) {
	if ttl <= 0 {
		ttl = 60
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	leaseResp, err := cli.Grant(ctx, ttl)
	if err != nil {
		cli.Close()
		cancel()
		return nil, fmt.Errorf("create lease failed: %v", err)
	}
	leaseID := leaseResp.ID

	key := fmt.Sprintf("/services/%s/%s", serviceName, serviceAddr)
	val := endpoints.Endpoint{
		Addr:     serviceAddr,
		Metadata: metadata,
	}

	registry := &Registry{
		client:  cli,
		leaseID: leaseID,
		ctx:     ctx,
		cancel:  cancel,
		key:     key,
		val:     val,
		ttl:     ttl,
	}
	registry.em, err = endpoints.NewManager(cli, serviceName)
	if err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(ctx, time.Second*2)
	defer cancel()
	err = registry.em.AddEndpoint(ctx, key, val)
	if err != nil {
		return nil, err
	}

	go registry.keepAlive()

	return registry, nil
}

// keepAlive auto-renewal
func (r *Registry) keepAlive() {
	keepAliveCh, err := r.client.KeepAlive(r.ctx, r.leaseID)
	if err != nil {
		log.Printf("create keep alive failed: %v", err)
		r.Unregister()
		return
	}

	for {
		select {
		case <-r.ctx.Done():
			return
		case resp := <-keepAliveCh:
			if resp == nil {
				log.Printf("lease has expired or been revoked, re-register for service")
				if err := r.reRegister(); err != nil {
					log.Printf("re-register service failed: %v", err)
					r.Unregister()
					return
				}
				return
			}
		}
	}
}

// reRegister re-register the service
func (r *Registry) reRegister() error {
	leaseResp, err := r.client.Grant(r.ctx, r.ttl)
	if err != nil {
		return fmt.Errorf("create lease failed: %v", err)
	}
	r.leaseID = leaseResp.ID

	ctx, cancel := context.WithTimeout(r.ctx, time.Second*2)
	defer cancel()
	err = r.em.AddEndpoint(ctx, r.key, r.val)
	if err != nil {
		return err
	}

	go r.keepAlive()

	return nil
}

// Unregister logout service
func (r *Registry) Unregister() error {
	r.cancel()

	_, err := r.client.Delete(context.Background(), r.key)
	if err != nil {
		log.Printf("delete service failed: %v", err)
	}

	_, err = r.client.Revoke(context.Background(), r.leaseID)
	if err != nil {
		log.Printf("revoke a lease failed: %v", err)
	}

	return r.client.Close()
}
