// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/ecstasoy/RPCinGo/pkg/registry"
)

// RoundRobinBalancer rotates through instances in order.
type RoundRobinBalancer struct {
	index uint64
}

// NewRoundRobin returns a round-robin balancer.
func NewRoundRobin() LoadBalancer {
	return &RoundRobinBalancer{}
}

// Pick returns the next instance in round-robin order.
func (rb *RoundRobinBalancer) Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("no available instances")
	}

	idx := atomic.AddUint64(&rb.index, 1) % uint64(len(instances))
	return instances[idx], nil
}

// Name returns the balancer name.
func (rb *RoundRobinBalancer) Name() string {
	return "round-robin"
}
