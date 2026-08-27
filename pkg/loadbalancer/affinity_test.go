package loadbalancer

import (
	"context"
	"testing"

	"RPCinGo/pkg/registry"
)

// TestConsistentHashAffinityReachable verifies that NewPickOptions makes the
// affinity key usable from outside the package: the same key maps to the same
// instance across calls, which was impossible while PickOptions.key was
// unexported.
func TestConsistentHashAffinityReachable(t *testing.T) {
	ch := NewConsistentHash()

	instances := []*registry.ServiceInstance{
		registry.NewServiceInstance("Svc", "10.0.0.1", 9000),
		registry.NewServiceInstance("Svc", "10.0.0.2", 9000),
		registry.NewServiceInstance("Svc", "10.0.0.3", 9000),
	}

	first, err := ch.PickWithOptions(context.Background(), instances, NewPickOptions("tenant-7"))
	if err != nil {
		t.Fatalf("pick: %v", err)
	}

	for i := 0; i < 20; i++ {
		got, err := ch.PickWithOptions(context.Background(), instances, NewPickOptions("tenant-7"))
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if got.ID != first.ID {
			t.Fatalf("affinity unstable: call %d picked %s, first picked %s", i, got.ID, first.ID)
		}
	}
}

// TestPickOptionsKeyAccessor confirms the exported accessors round-trip the key.
func TestPickOptionsKeyAccessor(t *testing.T) {
	if got := NewPickOptions("abc").Key(); got != "abc" {
		t.Errorf("Key() = %q, want abc", got)
	}
	if got := NewPickOptions("abc").WithKey("xyz").Key(); got != "xyz" {
		t.Errorf("WithKey().Key() = %q, want xyz", got)
	}
	var nilOpts *PickOptions
	if got := nilOpts.Key(); got != "" {
		t.Errorf("nil.Key() = %q, want empty", got)
	}
}
