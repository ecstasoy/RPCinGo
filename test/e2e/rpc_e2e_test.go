package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/ratelimiter"

	"github.com/ecstasoy/RPCinGo/pkg/client"
	"github.com/ecstasoy/RPCinGo/pkg/registry/memory"
	"github.com/ecstasoy/RPCinGo/pkg/server"
)

func TestE2E_ServiceDiscovery(t *testing.T) {
	memReg := memory.NewRegistry()
	defer memReg.Close()

	srv := server.NewServer(
		server.WithAddress("127.0.0.1:0"),
		server.WithRegistry("Calculator", "v1.0.0", memReg),
	)

	srv.RegisterMethod("Calculator", "Add", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		argsBytes, ok := req.Args.([]byte)
		if !ok {
			return nil, fmt.Errorf("args is not []byte, got %T", req.Args)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(argsBytes, &m); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}

		a := int(m["a"].(float64))
		b := int(m["b"].(float64))
		return a + b, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ✅ 使用 Start() 而不是手动 Listen + Serve
	go srv.Start(ctx)

	// 等待服务启动和注册完成
	time.Sleep(300 * time.Millisecond)

	// Verify registration
	instances, _ := memReg.GetInstances(context.Background(), "Calculator")
	t.Logf("Registered instances: %d", len(instances))
	if len(instances) == 0 {
		t.Fatal("Server should be registered")
	}

	// Create client with discovery
	cli, err := client.NewDiscoveryClient(
		client.WithDiscovery(memReg),
	)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer cli.Close()

	// RPC call
	resp, err := cli.Call(context.Background(), "Calculator", "Add",
		map[string]interface{}{"a": 10, "b": 20})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	if resp.IsError() {
		t.Fatalf("rpc error: %v", resp.Error)
	}

	dataBytes := resp.Data.([]byte)
	var sumVal float64
	if err := json.Unmarshal(dataBytes, &sumVal); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	sum := int(sumVal)
	if sum != 30 {
		t.Errorf("expected 30, got %d", sum)
	}

	t.Logf("✅ E2E test passed: 10 + 20 = %d", sum)

	srv.Stop()
}

func TestE2E_MultipleInstances(t *testing.T) {
	memReg := memory.NewRegistry()
	defer memReg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 3 servers
	servers := make([]*server.Server, 3)
	for i := 0; i < 3; i++ {
		srv := server.NewServer(
			server.WithAddress("127.0.0.1:0"),
			server.WithRegistry("Calculator", "v1.0.0", memReg),
		)

		serverID := i
		srv.RegisterMethod("Calculator", "GetID", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
			return serverID, nil
		})

		go srv.Start(ctx)

		servers[i] = srv
	}

	time.Sleep(300 * time.Millisecond)

	// Create client
	cli, _ := client.NewDiscoveryClient(
		client.WithDiscovery(memReg),
	)
	defer cli.Close()

	// Make multiple calls (should distribute)
	results := make(map[int]int)
	for i := 0; i < 9; i++ {
		resp, err := cli.Call(context.Background(), "Calculator", "GetID", nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}

		if resp.IsError() {
			t.Fatalf("rpc error: %v", resp.Error)
		}

		dataBytes := resp.Data.([]byte)
		var serverIDVal float64
		if err := json.Unmarshal(dataBytes, &serverIDVal); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}

		serverID := int(serverIDVal)
		results[serverID]++
	}

	t.Logf("Distribution: %v", results)

	if len(results) != 3 {
		t.Errorf("expected 3 servers, got %d", len(results))
	}

	t.Log("✅ Multiple instances test passed")

	// Cleanup
	for _, srv := range servers {
		srv.Stop()
	}
}

func TestE2E_WithMiddleware(t *testing.T) {
	memReg := memory.NewRegistry()
	defer memReg.Close()

	srv := server.NewServer(
		server.WithAddress("127.0.0.1:0"),
		server.WithRegistry("Calculator", "v1.0.0", memReg),
	)

	// Add middleware
	srv.Use(
		interceptor.Recovery(),
		interceptor.Logging(nil),
		interceptor.Metrics(),
	)

	srv.RegisterMethod("Calculator", "Add", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		argsBytes, ok := req.Args.([]byte)
		if !ok {
			return nil, fmt.Errorf("args is not []byte, got %T", req.Args)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(argsBytes, &m); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}

		a := int(m["a"].(float64))
		b := int(m["b"].(float64))
		return a + b, nil
	})

	srv.RegisterMethod("Calculator", "Panic", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		panic("intentional panic for testing")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Start(ctx)
	time.Sleep(300 * time.Millisecond)

	cli, _ := client.NewDiscoveryClient(
		client.WithDiscovery(memReg),
	)
	defer cli.Close()

	// Test 1: Normal call (should log)
	resp, err := cli.Call(context.Background(), "Calculator", "Add",
		map[string]interface{}{"a": 5, "b": 3})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if resp.IsError() {
		t.Fatalf("rpc error: %v", resp.Error)
	}

	dataBytes := resp.Data.([]byte)
	var sumVal float64
	if err := json.Unmarshal(dataBytes, &sumVal); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	sum := int(sumVal)
	if sum != 8 {
		t.Errorf("expected 8, got %d", sum)
	}

	t.Log("✅ Normal call with middleware passed")

	// Test 2: Panic call (should be recovered)
	_, err = cli.Call(context.Background(), "Calculator", "Panic", nil)
	if err == nil {
		t.Error("panic should be caught and returned as error")
	}

	t.Logf("✅ Panic recovered: %v", err)

	// Test 3: Verify server still alive after panic
	resp2, err := cli.Call(context.Background(), "Calculator", "Add",
		map[string]interface{}{"a": 1, "b": 2})
	if err != nil {
		t.Fatal("server should still be alive after panic")
	}

	if resp2.IsError() {
		t.Fatalf("rpc error: %v", resp2.Error)
	}

	t.Log("✅ Server survived panic (Recovery works!)")

	srv.Stop()
}

func TestE2E_RateLimit(t *testing.T) {
	memReg := memory.NewRegistry()
	defer memReg.Close()

	srv := server.NewServer(
		server.WithAddress("127.0.0.1:0"),
		server.WithRegistry("Calculator", "v1.0.0", memReg),
	)

	// Rate limit: 10 QPS
	rl := ratelimiter.NewTokenBucketLimiter(10, 10)

	srv.Use(
		interceptor.RateLimit(rl),
	)

	srv.RegisterMethod("Calculator", "Add", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		argsBytes, ok := req.Args.([]byte)
		if !ok {
			return nil, fmt.Errorf("args is not []byte, got %T", req.Args)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(argsBytes, &m); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}

		return int(m["a"].(float64)) + int(m["b"].(float64)), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Start(ctx)
	time.Sleep(300 * time.Millisecond)

	cli, _ := client.NewDiscoveryClient(
		client.WithDiscovery(memReg),
	)
	defer cli.Close()

	// Rapid fire 20 requests
	success := 0
	rateLimit := 0

	for i := 0; i < 20; i++ {
		_, err := cli.Call(context.Background(), "Calculator", "Add",
			map[string]interface{}{"a": 1, "b": 2})

		if err == nil {
			success++
		} else if errors.Is(err, ratelimiter.ErrRateLimitExceeded) {
			rateLimit++
		} else {
			t.Logf("unexpected err[%d]=%v", i, err)
		}
	}

	t.Logf("Success: %d, Rate Limited: %d", success, rateLimit)

	// First 10 should pass, rest rejected
	if success < 8 || success > 12 {
		t.Fatalf("expected ~10 success, got %d", success)
	}

	if rateLimit < 8 || rateLimit > 12 {
		t.Fatalf("expected ~10 rate limited, got %d", rateLimit)
	}

	t.Log("✅ Rate limit test passed")

	srv.Stop()
}
