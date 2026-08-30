package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/client"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry/memory"
)

func waitForAddr(t *testing.T, s *Server, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addr := s.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server addr not ready within %v", timeout)
	return ""
}

func TestServer_RegisterAndCall(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	srv.RegisterMethod("Calculator", "Add", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		argsBytes, ok := req.Args.([]byte)
		if !ok {
			argsMap := req.Args.(map[string]interface{})
			a := int(argsMap["a"].(float64))
			b := int(argsMap["b"].(float64))
			return a + b, nil
		}

		var argsMap map[string]interface{}
		if err := json.Unmarshal(argsBytes, &argsMap); err != nil {
			return nil, err
		}
		a := int(argsMap["a"].(float64))
		b := int(argsMap["b"].(float64))
		return a + b, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Stop()

	go func() {
		_ = srv.Start(ctx) // 测试里一般不要求打印 start 的 error
	}()

	addr := waitForAddr(t, srv, 1*time.Second)
	t.Logf("Server address: %s", addr)

	rpcClient, err := client.NewClient(addr)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer rpcClient.Close()

	resp, err := rpcClient.Call(context.Background(), "Calculator", "Add",
		map[string]interface{}{"a": 10, "b": 20})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if resp.IsError() {
		t.Fatalf("rpc error: %v", resp.Error)
	}

	if resp.Data == nil {
		t.Fatalf("resp.Data is nil")
	}

	t.Logf("resp.Data type: %T, DataCodec: %v", resp.Data, resp.DataCodec)

	dataBytes, ok := resp.Data.([]byte)
	if !ok {
		t.Fatalf("resp.Data is not []byte: %T", resp.Data)
	}

	t.Logf("dataBytes: %s", string(dataBytes))

	var data interface{}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatalf("unmarshal data failed: %v", err)
	}

	if data == nil {
		t.Fatalf("data is nil after unmarshal")
	}

	sum := int(data.(float64))
	if sum != 30 {
		t.Errorf("expected 30, got %d", sum)
	}
	t.Logf("✅ RPC call successful: 10 + 20 = %d", sum)
}

func TestServer_MultipleServices(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	srv.RegisterMethod("Calculator", "Add", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		argsBytes, _ := req.Args.([]byte)
		var m map[string]interface{}
		json.Unmarshal(argsBytes, &m)
		return int(m["a"].(float64)) + int(m["b"].(float64)), nil
	})
	srv.RegisterMethod("Greeter", "SayHello", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		argsBytes, _ := req.Args.([]byte)
		var m map[string]interface{}
		json.Unmarshal(argsBytes, &m)
		name := m["name"].(string)
		return fmt.Sprintf("Hello, %s!", name), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Stop()

	go func() { _ = srv.Start(ctx) }()
	addr := waitForAddr(t, srv, 1*time.Second)

	rpcClient, err := client.NewClient(addr)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer rpcClient.Close()

	result1, err := rpcClient.Call(context.Background(), "Calculator", "Add",
		map[string]interface{}{"a": 5, "b": 3})
	if err != nil {
		t.Fatalf("Calculator.Add failed: %v", err)
	}
	t.Logf("Calculator.Add(5, 3) = %v", result1)

	result2, err := rpcClient.Call(context.Background(), "Greeter", "SayHello",
		map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("Greeter.SayHello failed: %v", err)
	}
	t.Logf("Greeter.SayHello('Alice') = %v", result2)

	t.Log("✅ Multiple services test passed")
}

func TestServer_ServiceNotFound(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Stop()

	go func() { _ = srv.Start(ctx) }()
	addr := waitForAddr(t, srv, 1*time.Second)

	rpcClient, err := client.NewClient(addr)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer rpcClient.Close()

	_, err = rpcClient.Call(context.Background(), "NonExist", "Method", nil)
	if err == nil {
		t.Fatal("should return error for non-existent service")
	}

	t.Logf("✅ Correctly returned error: %v", err)
}

func TestServer_WithRegistry(t *testing.T) {
	memReg := memory.NewRegistry()
	defer memReg.Close()

	srv := NewServer(
		WithAddress("127.0.0.1:0"),
		WithRegistry("TestService", "v1.0.0", memReg),
	)

	srv.RegisterMethod("Echo", "Echo", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
		return req.Args, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = srv.Start(ctx) }()

	addr := waitForAddr(t, srv, 1*time.Second)
	t.Logf("Server address: %s", addr)

	// 等 registry 最多 1s
	deadline := time.Now().Add(1 * time.Second)
	for {
		instances, err := memReg.GetInstances(context.Background(), "TestService")
		if err == nil && len(instances) == 1 {
			t.Logf("✅ Service registered: %s", instances[0].Endpoint())
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("service not registered in time, instances=%v err=%v", instances, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Stop 后验证下线
	srv.Stop()
	time.Sleep(50 * time.Millisecond)

	instances, _ := memReg.GetInstances(context.Background(), "TestService")
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after stop, got %d", len(instances))
	}

	t.Log("✅ Server with Registry test passed")
}
