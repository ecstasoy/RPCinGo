// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"errors"

	"RPCinGo/pkg/registry"
)

var (
	ErrNoInstances      = errors.New("no available instances")
	ErrInvalidAlgorithm = errors.New("invalid algorithm")
)

type LoadBalancer interface {
	Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error)
	Name() string
}

type PickOptions struct {
	key      string
	Metadata map[string]string
}

type BalancerWithOptions interface {
	LoadBalancer
	PickWithOptions(ctx context.Context, instances []*registry.ServiceInstance, opts *PickOptions) (*registry.ServiceInstance, error)
}
