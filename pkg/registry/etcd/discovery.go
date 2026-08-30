// Kunhua Huang 2026

package etcd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecstasoy/RPCinGo/pkg/registry"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdDiscovery implements registry.Discovery on top of etcd key prefixes.
type EtcdDiscovery struct {
	*EtcdClient
}

// NewEtcdDiscovery constructs an etcd-backed discovery client.
func NewEtcdDiscovery(config *Config) (*EtcdDiscovery, error) {
	client, err := NewEtcdClient(config)
	if err != nil {
		return nil, err
	}

	return &EtcdDiscovery{
		EtcdClient: client,
	}, nil
}

// GetInstances returns all healthy instances registered under service.
func (ed *EtcdDiscovery) GetInstances(ctx context.Context, service string) ([]*registry.ServiceInstance, error) {
	prefix := ed.servicePrefix(service)

	resp, err := ed.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("get instances: %w", err)
	}

	var instances []*registry.ServiceInstance
	for _, kv := range resp.Kvs {
		var instance registry.ServiceInstance
		if err := json.Unmarshal(kv.Value, &instance); err != nil {
			fmt.Println("warning: unmarshal service instance failed:", err)
			continue
		}
		if instance.Status == registry.StatusUp {
			instances = append(instances, &instance)
		}
	}

	return instances, nil
}

// GetInstanceByID fetches one instance by its exact ID and requires it to be
// healthy.
func (ed *EtcdDiscovery) GetInstanceByID(ctx context.Context, service, instanceID string) (*registry.ServiceInstance, error) {
	key := ed.serviceKey(service, instanceID)

	resp, err := ed.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get instance by ID: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	var instance registry.ServiceInstance
	if err := json.Unmarshal(resp.Kvs[0].Value, &instance); err != nil {
		return nil, fmt.Errorf("unmarshal service instance: %w", err)
	}

	if instance.Status != registry.StatusUp {
		return nil, fmt.Errorf("instance is not up: %s", instanceID)
	}

	return &instance, nil
}

// Watch subscribes to instance changes for service and returns a Watcher that
// yields add, update, and delete events.
func (ed *EtcdDiscovery) Watch(ctx context.Context, service string) (registry.Watcher, error) {
	prefix := ed.servicePrefix(service)
	ctx, cancel := context.WithCancel(ctx)
	watchCh := ed.client.Watch(ctx, prefix, clientv3.WithPrefix(), clientv3.WithPrevKV())

	return &etcdWatcher{
		watchCh: watchCh,
		stopCh:  make(chan struct{}),
		cancel:  cancel,
	}, nil
}
