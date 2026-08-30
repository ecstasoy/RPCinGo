// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"math/rand"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/registry"
)

// Random picks a uniformly random instance from the available set.
type Random struct {
	rnd *rand.Rand
}

// NewRandom returns a random load balancer.
func NewRandom() LoadBalancer {
	return &Random{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Pick selects a random instance.
func (r *Random) Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}

	idx := r.rnd.Intn(len(instances))
	return instances[idx], nil
}

// Name returns the balancer name.
func (r *Random) Name() string {
	return "random"
}
