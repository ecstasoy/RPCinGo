package etcd

import (
	"context"
	"net"
	"testing"
	"time"

	"RPCinGo/pkg/registry"
)

// requireEtcd skips the test unless a real etcd is listening. NewEtcdRegistry
// dials lazily and returns no error when etcd is down, so the later Register or
// Watch call is what blocks — long past the point where skipping is possible.
// Probing the endpoint first is the only guard that actually keeps
// "go test ./..." usable on a machine without etcd.
func requireEtcd(t *testing.T, config *Config) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", config.Endpoints[0], 200*time.Millisecond)
	if err != nil {
		t.Skipf("etcd not available at %s: %v", config.Endpoints[0], err)
	}
	_ = conn.Close()
}

func TestEtcdRegistry_RegisterAndGet(t *testing.T) {
	config := DefaultConfig()
	requireEtcd(t, config)

	reg, err := NewEtcdRegistry(config)
	if err != nil {
		t.Skip("etcd not available:", err)
		return
	}
	defer reg.Close()

	disc, _ := NewEtcdDiscovery(config)
	defer disc.Close()

	instance := registry.NewServiceInstance("TestService", "localhost", 9999)

	err = reg.Register(context.Background(), instance)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	instances, err := disc.GetInstances(context.Background(), "TestService")
	if err != nil {
		t.Fatalf("get instances: %v", err)
	}

	if len(instances) == 0 {
		t.Error("no instances found")
	}

	t.Logf("✅ Found %d instance(s)", len(instances))

	reg.Deregister(context.Background(), "TestService", instance.ID)
}

func TestEtcdDiscovery_Watch(t *testing.T) {
	config := DefaultConfig()
	requireEtcd(t, config)

	disc, err := NewEtcdDiscovery(config)
	if err != nil {
		t.Skip("etcd not available:", err)
		return
	}
	defer disc.Close()

	watcher, err := disc.Watch(context.Background(), "WatchTest")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer watcher.Stop()

	go func() {
		time.Sleep(200 * time.Millisecond)

		reg, _ := NewEtcdRegistry(config)
		defer reg.Close()

		instance := registry.NewServiceInstance("WatchTest", "localhost", 8888)
		reg.Register(context.Background(), instance)
	}()

	event, err := watcher.Next()
	if err != nil {
		t.Fatalf("next event: %v", err)
	}

	if event.Type != registry.EventTypeAdd {
		t.Errorf("expected ADD, got %v", event.Type)
	}

	t.Log("✅ Watch test passed")
}
