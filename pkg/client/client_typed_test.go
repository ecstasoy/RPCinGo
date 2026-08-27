package client

import (
	"context"
	"testing"
	"time"

	"RPCinGo/examples/proto/calculator"
	"RPCinGo/pkg/server"
)

func waitForAddrClient(t *testing.T, s *server.Server, timeout time.Duration) string {
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

type CalculatorService struct{}

func (s *CalculatorService) Add(
	ctx context.Context,
	req *calculator.AddRequest,
) (*calculator.AddResponse, error) {
	return &calculator.AddResponse{
		Result: req.A + req.B,
	}, nil
}

func (s *CalculatorService) Subtract(
	ctx context.Context,
	req *calculator.SubtractRequest,
) (*calculator.SubtractResponse, error) {
	return &calculator.SubtractResponse{
		Result: req.A - req.B,
	}, nil
}

func TestClient_CallTyped(t *testing.T) {
	srv := server.NewServer(server.WithAddress("127.0.0.1:0"))

	calcService := &CalculatorService{}
	if err := srv.RegisterService("Calculator", calcService); err != nil {
		t.Fatalf("RegisterService failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Stop()

	go func() {
		_ = srv.Start(ctx)
	}()

	addr := waitForAddrClient(t, srv, 1*time.Second)
	t.Logf("Server address: %s", addr)

	cli, err := NewClient(addr)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer cli.Close()

	req := &calculator.AddRequest{A: 10, B: 20}
	resp := &calculator.AddResponse{}

	_, err = cli.CallTyped(ctx, "Calculator", "Add", req, resp)
	if err != nil {
		t.Fatalf("CallTyped failed: %v", err)
	}

	if resp.Result != 30 {
		t.Errorf("expected 30, got %d", resp.Result)
	}
	t.Logf("✅ Typed RPC call successful: 10 + 20 = %d", resp.Result)

	req2 := &calculator.SubtractRequest{A: 50, B: 20}
	resp2 := &calculator.SubtractResponse{}

	_, err = cli.CallTyped(ctx, "Calculator", "Subtract", req2, resp2)
	if err != nil {
		t.Fatalf("CallTyped failed: %v", err)
	}

	if resp2.Result != 30 {
		t.Errorf("expected 30, got %d", resp2.Result)
	}
	t.Logf("✅ Typed RPC call successful: 50 - 20 = %d", resp2.Result)
}
