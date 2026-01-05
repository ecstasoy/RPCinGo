// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"sync"

	"RPCinGo/pkg/registry"
)

type WeightedRoundRobin struct {
	instances []*registry.ServiceInstance
	weights   []int

	current int // current index in weights
	mu      sync.Mutex
}

func NewWeightedRoundRobin() LoadBalancer {
	return &WeightedRoundRobin{}
}

func (wrr *WeightedRoundRobin) Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}

	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	if !wrr.isSameInstances(instances) {
		wrr.rebuild(instances)
	}

	if len(wrr.weights) == 0 {
		return instances[0], nil
	}

	wrr.current = (wrr.current + 1) % len(wrr.weights)
	idx := wrr.weights[wrr.current]

	return wrr.instances[idx], nil
}

func (wrr *WeightedRoundRobin) rebuild(instances []*registry.ServiceInstance) {
	wrr.instances = instances
	wrr.weights = wrr.weights[:0]

	for i, inst := range instances {
		weight := inst.Weight
		if weight <= 0 {
			weight = 100
		}

		for j := 0; j < weight; j++ {
			wrr.weights = append(wrr.weights, i)
		}
	}

	wrr.current = -1
}

func (wrr *WeightedRoundRobin) isSameInstances(instances []*registry.ServiceInstance) bool {
	if len(instances) != len(wrr.instances) {
		return false
	}

	for i, inst := range instances {
		if inst.ID != wrr.instances[i].ID {
			return false
		}
	}

	return true
}

func (wrr *WeightedRoundRobin) Name() string {
	return "weighted-round-robin"
}
