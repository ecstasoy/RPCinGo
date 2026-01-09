// Kunhua Huang 2026

package etcd

import (
	"context"
	"encoding/json"
	"fmt"

	"RPCinGo/pkg/registry"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdDiscovery struct {
	*EtcdClient
}

func NewEtcdDiscovery(config *Config) (*EtcdDiscovery, error) {
	client, err := NewEtcdClient(config)
	if err != nil {
		return nil, err
	}

	return &EtcdDiscovery{
		EtcdClient: client,
	}, nil
}

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
