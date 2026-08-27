package interceptor

import (
	"RPCinGo/pkg/protocol"
	"context"
	"testing"
)

func TestRecovery(t *testing.T) {
	recovery := Recovery()

	panicHandler := func(ctx context.Context, req *protocol.Request) (any, error) {
		panic("test panic")
	}

	resp, err := recovery(context.Background(), nil, panicHandler)

	if err == nil {
		t.Error("should catch panic and return error")
	}

	if resp != nil {
		t.Error("response should be nil on panic")
	}

	t.Logf("✅ Recovery caught panic: %v", err)
}

func TestLogging(t *testing.T) {
	logging := Logging(nil)

	handler := func(ctx context.Context, req *protocol.Request) (any, error) {
		return "ok", nil
	}

	resp, err := logging(context.Background(), &protocol.Request{
		Service: "TestService",
		Method:  "TestMethod",
	}, handler)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp != "ok" {
		t.Error("response mismatch")
	}

	t.Log("✅ Logging test passed")
}

func TestInterceptorChain(t *testing.T) {
	callOrder := []string{}

	interceptor1 := func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		callOrder = append(callOrder, "1-before")
		resp, err := invoker(ctx, req)
		callOrder = append(callOrder, "1-after")
		return resp, err
	}

	interceptor2 := func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		callOrder = append(callOrder, "2-before")
		resp, err := invoker(ctx, req)
		callOrder = append(callOrder, "2-after")
		return resp, err
	}

	handler := func(ctx context.Context, req *protocol.Request) (any, error) {
		callOrder = append(callOrder, "handler")
		return "result", nil
	}

	chain := NewChain(interceptor1, interceptor2)
	chain.Intercept(context.Background(), nil, handler)

	expected := []string{"1-before", "2-before", "handler", "2-after", "1-after"}

	for i, call := range callOrder {
		if call != expected[i] {
			t.Errorf("order[%d]: expected %s, got %s", i, expected[i], call)
		}
	}

	t.Logf("Call order: %v", callOrder)
	t.Log("✅ Chain test passed")
}
