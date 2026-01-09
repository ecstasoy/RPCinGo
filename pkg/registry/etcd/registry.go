// Kunhua Huang 2026

package etcd

import (
	"context"
	"encoding/json"
	"fmt"

	"RPCinGo/pkg/registry"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdRegistry struct {
	*EtcdClient
	leaseID clientv3.LeaseID

	keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse
	stopCh      chan struct{}
}

func NewEtcdRegistry(config *Config) (*EtcdRegistry, error) {
	client, err := NewEtcdClient(config)
	if err != nil {
		return nil, fmt.Errorf("create etcd client error: %v", err)
	}

	er := &EtcdRegistry{
		EtcdClient: client,
		stopCh:     make(chan struct{}),
	}

	if err := er.createLease(context.Background()); err != nil {
		return nil, fmt.Errorf("create lease error: %v", err)
	}

	return er, nil
}

func (er *EtcdRegistry) createLease(ctx context.Context) error {
	grant, err := er.client.Grant(ctx, er.config.LeaseTTL)
	if err != nil {
		return fmt.Errorf("grant lease: %w", err)
	}

	er.leaseID = grant.ID

	keepAliveCh, err := er.client.KeepAlive(context.Background(), er.leaseID)
	if err != nil {
		return fmt.Errorf("keep alive lease: %w", err)
	}

	er.keepAliveCh = keepAliveCh

	go er.listenKeepAlive()

	return nil
}

func (er *EtcdRegistry) listenKeepAlive() {
	for {
		select {
		case <-er.stopCh:
			return
		case resp, ok := <-er.keepAliveCh:
			if !ok {
				return
			}
			if resp == nil {
				return
			}
		}
	}
}

func (er *EtcdRegistry) Register(ctx context.Context, instance *registry.ServiceInstance) error {
	key := er.serviceKey(instance.Service, instance.ID)

	value, err := json.Marshal(instance)
	if err != nil {
		return fmt.Errorf("marshal instance: %w", err)
	}

	_, err = er.client.Put(ctx, key, string(value), clientv3.WithLease(er.leaseID))
	if err != nil {
		return fmt.Errorf("put to etcd: %w", err)
	}

	return nil
}

func (er *EtcdRegistry) Deregister(ctx context.Context, service, instanceID string) error {
	key := er.serviceKey(service, instanceID)

	_, err := er.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("delete service instance: %v from etcd: %v", instanceID, err)
	}

	return nil
}

func (er *EtcdRegistry) Heartbeat(ctx context.Context, service, instanceID string) error {
	_, err := er.client.KeepAliveOnce(ctx, er.leaseID)
	if err != nil {
		return fmt.Errorf("keepalive once: %w", err)
	}
	return nil
}

func (er *EtcdRegistry) Update(ctx context.Context, instance *registry.ServiceInstance) error {
	return er.Register(ctx, instance)
}

func (er *EtcdRegistry) Close() error {
	close(er.stopCh)

	if er.leaseID != 0 {
		er.client.Revoke(context.Background(), er.leaseID)
	}

	return er.EtcdClient.Close()
}
