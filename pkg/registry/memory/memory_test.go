package memory

import (
	"context"
	"testing"

	"RPCinGo/pkg/registry"
)

func TestMemoryRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	defer reg.Close()

	instance := registry.NewServiceInstance("UserService", "localhost", 8080)

	err := reg.Register(context.Background(), instance)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	instances, err := reg.GetInstances(context.Background(), "UserService")
	if err != nil {
		t.Fatalf("get instances failed: %v", err)
	}

	if len(instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(instances))
	}

	if instances[0].Service != "UserService" {
		t.Error("service name mismatch")
	}

	t.Log("✅ Register and Get test passed")
}

func TestMemoryRegistry_Watch(t *testing.T) {
	reg := NewRegistry()
	defer reg.Close()

	ctx := context.Background()

	watcher, err := reg.Watch(ctx, "TestService")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	defer watcher.Stop()

	go func() {
		instance := registry.NewServiceInstance("TestService", "localhost", 9090)
		reg.Register(ctx, instance)
	}()

	event, err := watcher.Next()
	if err != nil {
		t.Fatalf("next event failed: %v", err)
	}

	if event.Type != registry.EventTypeAdd {
		t.Errorf("expected ADD event, got %v", event.Type)
	}

	t.Log("✅ Watch test passed")
}
