// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"fmt"
	"sync/atomic"

	"RPCinGo/pkg/registry"
)

type RoundRobinBalancer struct {
	index uint64
}

func NewRoundRobin() LoadBalancer {
	return &RoundRobinBalancer{}
}

func (rb *RoundRobinBalancer) Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("no available instances")
	}

	idx := atomic.AddUint64(&rb.index, 1) % uint64(len(instances))
	return instances[idx], nil
}

func (rb *RoundRobinBalancer) Name() string {
	return "round-robin"
}
