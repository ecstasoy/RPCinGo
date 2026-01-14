// Kunhua Huang 2026
// In-memory implementation of service registry and discovery
// Used for testing and development purposes

package memory

import (
	"context"
	"sync"
	"time"

	"RPCinGo/pkg/registry"
)

type Registry struct {
	instances map[string]*registry.ServiceInstance
	watchers  map[string][]chan *registry.Event
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		instances: make(map[string]*registry.ServiceInstance),
		watchers:  make(map[string][]chan *registry.Event),
	}
}

func (r *Registry) Register(ctx context.Context, instance *registry.ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instances[instance.ID] = instance

	r.notify(instance.Service, &registry.Event{
		Type:     registry.EventTypeAdd,
		Instance: instance,
	})

	return nil
}

func (r *Registry) Deregister(ctx context.Context, service, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, exists := r.instances[instanceID]
	if !exists {
		return registry.ErrNotFound
	}

	delete(r.instances, instanceID)

	r.notify(instance.Service, &registry.Event{
		Type:     registry.EventTypeDelete,
		Instance: instance,
	})

	return nil
}

func (r *Registry) Update(ctx context.Context, instance *registry.ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.instances[instance.ID]; !exists {
		return registry.ErrNotFound
	}

	instance.UpdateTime = time.Now()
	r.instances[instance.ID] = instance

	r.notify(instance.Service, &registry.Event{
		Type:     registry.EventTypeUpdate,
		Instance: instance,
	})

	return nil
}

func (r *Registry) Heartbeat(ctx context.Context, service, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, exists := r.instances[instanceID]
	if !exists {
		return registry.ErrNotFound
	}

	instance.UpdateTime = time.Now()
	return nil
}

func (r *Registry) GetInstances(ctx context.Context, service string) ([]*registry.ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*registry.ServiceInstance
	for _, instance := range r.instances {
		if instance.Service == service && instance.Status == registry.StatusUp {
			result = append(result, instance)
		}
	}

	return result, nil
}

func (r *Registry) GetInstanceByID(ctx context.Context, instanceID string) (*registry.ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, exists := r.instances[instanceID]
	if !exists {
		return nil, registry.ErrNotFound
	}

	return instance, nil
}

func (r *Registry) Watch(ctx context.Context, service string) (registry.Watcher, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *registry.Event, 10)
	r.watchers[service] = append(r.watchers[service], ch)

	return &memoryWatcher{ch: ch}, nil
}

func (r *Registry) notify(service string, event *registry.Event) {
	watchers := r.watchers[service]
	for _, ch := range watchers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, watchers := range r.watchers {
		for _, ch := range watchers {
			close(ch)
		}
	}

	return nil
}

type memoryWatcher struct {
	ch     chan *registry.Event
	once   sync.Once
	stopCh chan struct{}
}

func (w *memoryWatcher) Next() (*registry.Event, error) {
	select {
	case <-w.stopCh:
		return nil, registry.ErrWatcherStopped
	case event, ok := <-w.ch:
		if !ok {
			return nil, registry.ErrWatcherStopped
		}
		return event, nil
	}
}

func (w *memoryWatcher) Stop() {
	w.once.Do(func() { close(w.stopCh) })
}
