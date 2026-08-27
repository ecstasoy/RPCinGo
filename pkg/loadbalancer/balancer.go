// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"errors"

	"RPCinGo/pkg/registry"
)

// ErrNoInstances and ErrInvalidAlgorithm are shared by load balancer
// implementations.
var (
	ErrNoInstances      = errors.New("no available instances")
	ErrInvalidAlgorithm = errors.New("invalid algorithm")
)

// LoadBalancer selects one service instance from the available set.
type LoadBalancer interface {
	Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error)
	Name() string
}

// PickOptions carries optional hints for balancers that support richer
// selection semantics.
type PickOptions struct {
	key      string
	Metadata map[string]string
}

// NewPickOptions builds PickOptions with the affinity key set. Callers use this
// to drive consistent-hash selection — without it the key field is unreachable
// from outside the package, stranding the affinity capability.
func NewPickOptions(key string) *PickOptions {
	return &PickOptions{key: key}
}

// Key returns the affinity key carried by the options.
func (o *PickOptions) Key() string {
	if o == nil {
		return ""
	}
	return o.key
}

// WithKey returns a copy of the options with the affinity key set.
func (o *PickOptions) WithKey(key string) *PickOptions {
	if o == nil {
		return &PickOptions{key: key}
	}
	clone := *o
	clone.key = key
	return &clone
}

// BalancerWithOptions extends LoadBalancer with optional per-pick hints.
type BalancerWithOptions interface {
	LoadBalancer
	PickWithOptions(ctx context.Context, instances []*registry.ServiceInstance, opts *PickOptions) (*registry.ServiceInstance, error)
}
