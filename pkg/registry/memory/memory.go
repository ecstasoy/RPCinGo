// Kunhua Huang 2026
// In-memory implementation of service registry and discovery
// Used for testing and development purposes

package memory

import (
	"context"
	"sync"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/registry"
)

// Registry is an in-memory implementation of registry.Registry and
// registry.Discovery.
type Registry struct {
	instances map[string]*registry.ServiceInstance
	watchers  map[string][]chan *registry.Event
	mu        sync.RWMutex
}

// NewRegistry returns an empty in-memory registry.
func NewRegistry() *Registry {
	return &Registry{
		instances: make(map[string]*registry.ServiceInstance),
		watchers:  make(map[string][]chan *registry.Event),
	}
}

// Register stores instance and notifies watchers of an add event.
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

// Deregister removes one instance and notifies watchers of a delete event.
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

// Update replaces an existing instance and notifies watchers of an update
// event.
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

// Heartbeat refreshes the instance update timestamp.
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

// GetInstances returns all healthy instances registered under service.
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

// GetInstanceByID returns the instance with instanceID.
func (r *Registry) GetInstanceByID(ctx context.Context, instanceID string) (*registry.ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, exists := r.instances[instanceID]
	if !exists {
		return nil, registry.ErrNotFound
	}

	return instance, nil
}

// Watch subscribes to change events for service.
func (r *Registry) Watch(ctx context.Context, service string) (registry.Watcher, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *registry.Event, 10)
	r.watchers[service] = append(r.watchers[service], ch)

	return &memoryWatcher{
		ch:     ch,
		stopCh: make(chan struct{}),
	}, nil
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

// Close closes all watcher channels managed by the registry.
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
