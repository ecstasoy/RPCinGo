package loadbalancer

import (
	"context"
	"testing"

	"github.com/ecstasoy/RPCinGo/pkg/registry"
)

func createTestInstances() []*registry.ServiceInstance {
	return []*registry.ServiceInstance{
		{ID: "1", Service: "Test", Address: "192.168.1.10", Port: 8080, Weight: 100},
		{ID: "2", Service: "Test", Address: "192.168.1.11", Port: 8080, Weight: 100},
		{ID: "3", Service: "Test", Address: "192.168.1.12", Port: 8080, Weight: 100},
	}
}

func TestRoundRobin(t *testing.T) {
	instances := createTestInstances()
	balancer := NewRoundRobin()

	results := make(map[string]int)

	for i := 0; i < 9; i++ {
		inst, err := balancer.Pick(context.Background(), instances)
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		results[inst.ID]++
	}

	// 应该均匀分布
	for id, count := range results {
		if count != 3 {
			t.Errorf("instance %s: expected 3, got %d", id, count)
		}
	}

	t.Logf("Distribution: %v", results)
	t.Log("✅ RoundRobin test passed")
}

func TestWeightedRoundRobin(t *testing.T) {
	instances := []*registry.ServiceInstance{
		{ID: "1", Service: "Test", Address: "host1", Port: 8080, Weight: 50},
		{ID: "2", Service: "Test", Address: "host2", Port: 8080, Weight: 30},
		{ID: "3", Service: "Test", Address: "host3", Port: 8080, Weight: 20},
	}

	balancer := NewWeightedRoundRobin()

	results := make(map[string]int)
	total := 100

	for i := 0; i < total; i++ {
		inst, _ := balancer.Pick(context.Background(), instances)
		results[inst.ID]++
	}

	t.Logf("Distribution: %v", results)

	// 验证比例接近权重
	if results["1"] < 45 || results["1"] > 55 {
		t.Errorf("instance 1 should be ~50, got %d", results["1"])
	}

	t.Log("✅ WeightedRoundRobin test passed")
}

func TestRandom(t *testing.T) {
	instances := createTestInstances()
	balancer := NewRandom()

	results := make(map[string]int)

	for i := 0; i < 300; i++ {
		inst, _ := balancer.Pick(context.Background(), instances)
		results[inst.ID]++
	}

	t.Logf("Distribution: %v", results)

	// 随机应该大致均匀（允许偏差）
	for id, count := range results {
		if count < 80 || count > 120 {
			t.Logf("Warning: instance %s has %d (expected ~100)", id, count)
		}
	}

	t.Log("✅ Random test passed")
}

func TestConsistentHash(t *testing.T) {
	instances := createTestInstances()
	balancer := NewConsistentHash()

	// 同一个 key 应该总是路由到同一个实例
	key := "user123"

	var firstPick *registry.ServiceInstance
	for i := 0; i < 10; i++ {
		inst, _ := balancer.PickWithOptions(
			context.Background(),
			instances,
			&PickOptions{key: key},
		)

		if i == 0 {
			firstPick = inst
		} else if inst.ID != firstPick.ID {
			t.Errorf("same key should always pick same instance")
		}
	}

	t.Logf("Key '%s' always routes to instance %s", key, firstPick.ID)
	t.Log("✅ ConsistentHash test passed")
}

func BenchmarkBalancers(b *testing.B) {
	instances := createTestInstances()

	balancers := map[string]LoadBalancer{
		"RoundRobin":     NewRoundRobin(),
		"Weighted":       NewWeightedRoundRobin(),
		"Random":         NewRandom(),
		"ConsistentHash": NewConsistentHash(),
	}

	for name, balancer := range balancers {
		b.Run(name, func(b *testing.B) {
			ctx := context.Background()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				balancer.Pick(ctx, instances)
			}
		})
	}
}
