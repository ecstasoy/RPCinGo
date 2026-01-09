// Kunhua Huang 2026

package registry

import (
	"context"
	"errors"
)

var (
	ErrNotFound       = errors.New("service not found")
	ErrAlreadyExists  = errors.New("service already exists")
	ErrNotConnected   = errors.New("not connected to registry")
	ErrWatcherStopped = errors.New("watcher has been stopped")
)

type Registry interface {
	Register(ctx context.Context, instance *ServiceInstance) error
	Deregister(ctx context.Context, service, instanceID string) error
	Update(ctx context.Context, instance *ServiceInstance) error
	Heartbeat(ctx context.Context, service, instanceID string) error
	Close() error
}

type Discovery interface {
	GetInstances(ctx context.Context, service string) ([]*ServiceInstance, error)
	Watch(ctx context.Context, service string) (Watcher, error)
	Close() error
}

type RegistryDiscovery interface {
	Registry
	Discovery
}
