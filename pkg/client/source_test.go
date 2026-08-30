package client

import (
	"context"
	"testing"

	"github.com/ecstasoy/RPCinGo/pkg/loadbalancer"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry"
	"github.com/ecstasoy/RPCinGo/pkg/registry/memory"
)

// recordingBalancer is a fake BalancerWithOptions that records which selection
// path (plain Pick vs PickWithOptions) was taken and the affinity key it saw.
type recordingBalancer struct {
	pickCalled bool
	optsCalled bool
	lastKey    string
}

func (b *recordingBalancer) Name() string { return "recording" }

func (b *recordingBalancer) Pick(_ context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	b.pickCalled = true
	return instances[0], nil
}

func (b *recordingBalancer) PickWithOptions(_ context.Context, instances []*registry.ServiceInstance, opts *loadbalancer.PickOptions) (*registry.ServiceInstance, error) {
	b.optsCalled = true
	b.lastKey = opts.Key()
	return instances[0], nil
}

func newDiscoverySourceWithBalancer(b loadbalancer.LoadBalancer) *discoverySource {
	return &discoverySource{
		loadBalancer:  b,
		instanceCache: map[string][]*registry.ServiceInstance{},
		watchers:      map[string]registry.Watcher{},
	}
}

// TestPickUsesHashKeyWhenPresent verifies the discovery source threads a request
// affinity key into the option-aware balancer — the capability that was
// previously unreachable because PickOptions.key was unexported.
func TestPickUsesHashKeyWhenPresent(t *testing.T) {
	b := &recordingBalancer{}
	s := newDiscoverySourceWithBalancer(b)

	instances := []*registry.ServiceInstance{
		registry.NewServiceInstance("Svc", "10.0.0.1", 9000),
	}

	req := protocol.NewRequest("Svc", "M", nil)
	req.SetMetadata(protocol.MetaKeyHashKey, "user-42")

	if _, err := s.pick(context.Background(), req, instances); err != nil {
		t.Fatalf("pick: %v", err)
	}
	if !b.optsCalled {
		t.Fatal("expected PickWithOptions to be used when a hash key is present")
	}
	if b.lastKey != "user-42" {
		t.Errorf("affinity key = %q, want user-42", b.lastKey)
	}
}

// TestPickFallsBackWithoutHashKey verifies that without an affinity key the
// source uses the plain Pick path.
func TestPickFallsBackWithoutHashKey(t *testing.T) {
	b := &recordingBalancer{}
	s := newDiscoverySourceWithBalancer(b)

	instances := []*registry.ServiceInstance{
		registry.NewServiceInstance("Svc", "10.0.0.1", 9000),
	}

	req := protocol.NewRequest("Svc", "M", nil)

	if _, err := s.pick(context.Background(), req, instances); err != nil {
		t.Fatalf("pick: %v", err)
	}
	if b.optsCalled {
		t.Fatal("did not expect PickWithOptions without a hash key")
	}
	if !b.pickCalled {
		t.Fatal("expected plain Pick to be used")
	}
}

// TestNewClientRejectsDiscoveryOptions verifies the fixed-address constructor
// fails loudly when handed a discovery backend instead of silently ignoring it.
func TestNewClientRejectsDiscoveryOptions(t *testing.T) {
	_, err := NewClient("127.0.0.1:9000", WithDiscovery(memory.NewRegistry()))
	if err == nil {
		t.Fatal("expected NewClient to reject WithDiscovery, got nil error")
	}
}
