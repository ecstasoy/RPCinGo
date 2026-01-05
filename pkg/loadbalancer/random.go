// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"math/rand"
	"time"

	"RPCinGo/pkg/registry"
)

type Random struct {
	rnd *rand.Rand
}

func NewRandom() LoadBalancer {
	return &Random{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *Random) Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}

	idx := r.rnd.Intn(len(instances))
	return instances[idx], nil
}

func (r *Random) Name() string {
	return "random"
}
