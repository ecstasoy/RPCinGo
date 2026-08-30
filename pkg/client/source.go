package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/ecstasoy/RPCinGo/pkg/loadbalancer"
	"github.com/ecstasoy/RPCinGo/pkg/pool"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry"
)

// connSource is the seam between Client.Call and the two ways a connection is
// obtained: a single fixed-address pool, or discovery + load balancing across
// per-endpoint pools. Two adapters (fixedSource, discoverySource) satisfy it, so
// Call has one code path regardless of mode — the mode lives behind the seam,
// not in a branch the caller has to reason about.
type connSource interface {
	// acquire returns a pooled connection ready to carry req. The caller owns
	// the returned connection: Release it on success, Close it on failure.
	acquire(ctx context.Context, req *protocol.Request) (*pool.PooledConnection, error)
	// Close releases the resources held by the source.
	Close() error
}

// fixedSource serves every request from one pool bound to a fixed address.
type fixedSource struct {
	pool *pool.ConnectionPool
}

func (s *fixedSource) acquire(ctx context.Context, _ *protocol.Request) (*pool.PooledConnection, error) {
	conn, err := s.pool.GetWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}
	return conn, nil
}

func (s *fixedSource) Close() error {
	return s.pool.Close()
}

// discoverySource resolves instances via discovery, picks one with the load
// balancer, and serves connections from a per-endpoint pool manager. It owns the
// instance cache and the background watch goroutines, so all discovery logic is
// concentrated here rather than spread across the Client.
type discoverySource struct {
	poolManager  *pool.PoolManager
	discovery    registry.Discovery
	loadBalancer loadbalancer.LoadBalancer
	enableWatch  bool

	instanceCache map[string][]*registry.ServiceInstance
	cacheMu       sync.RWMutex

	watchers map[string]registry.Watcher
	watchMu  sync.Mutex
}

func (s *discoverySource) acquire(ctx context.Context, req *protocol.Request) (*pool.PooledConnection, error) {
	instances, err := s.getInstances(ctx, req.Service)
	if err != nil {
		return nil, fmt.Errorf("get instances: %w", err)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no available instances for %s", req.Service)
	}

	instance, err := s.pick(ctx, req, instances)
	if err != nil {
		return nil, fmt.Errorf("pick instance: %w", err)
	}

	conn, err := s.poolManager.GetConnection(ctx, instance.Endpoint())
	if err != nil {
		return nil, fmt.Errorf("get connection to %s: %w", instance.Endpoint(), err)
	}

	return conn, nil
}

// pick selects an instance, honouring a hash-affinity key from request metadata
// when the load balancer supports option-based selection. Without a key (or a
// plain balancer) it falls back to the standard Pick.
func (s *discoverySource) pick(ctx context.Context, req *protocol.Request, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if bwo, ok := s.loadBalancer.(loadbalancer.BalancerWithOptions); ok {
		if key, has := req.GetMetadata(protocol.MetaKeyHashKey); has && key != "" {
			return bwo.PickWithOptions(ctx, instances, loadbalancer.NewPickOptions(key))
		}
	}
	return s.loadBalancer.Pick(ctx, instances)
}

func (s *discoverySource) Close() error {
	s.watchMu.Lock()
	for _, watcher := range s.watchers {
		watcher.Stop()
	}
	s.watchMu.Unlock()

	return s.poolManager.Close()
}

func (s *discoverySource) getInstances(ctx context.Context, service string) ([]*registry.ServiceInstance, error) {
	s.cacheMu.RLock()
	cached, ok := s.instanceCache[service]
	s.cacheMu.RUnlock()

	if ok && len(cached) > 0 {
		return cached, nil
	}

	if s.discovery == nil {
		return nil, fmt.Errorf("no discovery configured")
	}

	instances, err := s.discovery.GetInstances(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("discovery get instances: %w", err)
	}

	s.cacheMu.Lock()
	s.instanceCache[service] = instances
	s.cacheMu.Unlock()

	if s.enableWatch {
		go s.watchService(service)
	}

	return instances, nil
}

func (s *discoverySource) watchService(service string) {
	s.watchMu.Lock()
	if _, watching := s.watchers[service]; watching {
		s.watchMu.Unlock()
		return
	}

	watcher, err := s.discovery.Watch(context.Background(), service)
	if err != nil {
		s.watchMu.Unlock()
		return
	}

	s.watchers[service] = watcher
	s.watchMu.Unlock()

	for {
		event, err := watcher.Next()
		if err != nil {
			return
		}

		s.handleWatchEvent(service, event)
	}
}

func (s *discoverySource) handleWatchEvent(service string, event *registry.Event) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	instances := s.instanceCache[service]

	switch event.Type {
	case registry.EventTypeAdd:
		instances = append(instances, event.Instance)
	case registry.EventTypeDelete:
		filtered := make([]*registry.ServiceInstance, 0, len(instances))
		for _, inst := range instances {
			if inst.ID != event.Instance.ID {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered

		s.poolManager.RemovePool(event.Instance.Endpoint())
	case registry.EventTypeUpdate:
		for i, inst := range instances {
			if inst.ID == event.Instance.ID {
				instances[i] = event.Instance
				break
			}
		}
	}

	s.instanceCache[service] = instances
}
