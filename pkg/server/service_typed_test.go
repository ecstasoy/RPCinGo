package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"RPCinGo/examples/proto/calculator"
	"RPCinGo/pkg/client"
	"RPCinGo/pkg/protocol"

	"google.golang.org/protobuf/proto"
)

func waitForAddrTyped(t *testing.T, s *Server, timeout time.Duration) string {
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

func (s *CalculatorService) Add(ctx context.Context, req *calculator.AddRequest) (*calculator.AddResponse, error) {
	return &calculator.AddResponse{
		Result: req.A + req.B,
	}, nil
}

func (s *CalculatorService) Subtract(ctx context.Context, req *calculator.SubtractRequest) (*calculator.SubtractResponse, error) {
	return &calculator.SubtractResponse{
		Result: req.A - req.B,
	}, nil
}

func TestServer_RegisterService_TypedMethods(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	calcService := &CalculatorService{}
	if err := srv.RegisterService("Calculator", calcService); err != nil {
		t.Fatalf("RegisterService failed: %v", err)
	}

	t.Logf("✅ Successfully registered Calculator service with typed methods")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Stop()

	go func() {
		_ = srv.Start(ctx)
	}()

	addr := waitForAddrTyped(t, srv, 1*time.Second)
	t.Logf("Server address: %s", addr)

	rpcClient, err := client.NewClient(addr)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer rpcClient.Close()

	resp, err := rpcClient.Call(ctx, "Calculator", "Add",
		map[string]interface{}{"a": 10, "b": 20})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if resp.IsError() {
		t.Fatalf("rpc error: %v", resp.Error)
	}

	t.Logf("DataCodec: %v", resp.DataCodec)

	dataBytes := resp.Data.([]byte)
	addResp := &calculator.AddResponse{}

	switch resp.DataCodec {
	case protocol.PayloadCodecProtobuf:
		if err := proto.Unmarshal(dataBytes, addResp); err != nil {
			t.Fatalf("unmarshal protobuf result failed: %v", err)
		}
	case protocol.PayloadCodecJSON:
		if err := json.Unmarshal(dataBytes, addResp); err != nil {
			t.Fatalf("unmarshal json result failed: %v", err)
		}
	default:
		t.Fatalf("unsupported data codec: %v", resp.DataCodec)
	}

	sum := int(addResp.Result)
	if sum != 30 {
		t.Errorf("expected 30, got %d", sum)
	}
	t.Logf("✅ Typed RPC call successful: 10 + 20 = %d", sum)

	resp2, err := rpcClient.Call(ctx, "Calculator", "Subtract",
		map[string]interface{}{"a": 50, "b": 20})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if resp2.IsError() {
		t.Fatalf("rpc error: %v", resp2.Error)
	}

	dataBytes2 := resp2.Data.([]byte)
	subtractResp := &calculator.SubtractResponse{}

	switch resp2.DataCodec {
	case protocol.PayloadCodecProtobuf:
		if err := proto.Unmarshal(dataBytes2, subtractResp); err != nil {
			t.Fatalf("unmarshal protobuf result failed: %v", err)
		}
	case protocol.PayloadCodecJSON:
		if err := json.Unmarshal(dataBytes2, subtractResp); err != nil {
			t.Fatalf("unmarshal json result failed: %v", err)
		}
	default:
		t.Fatalf("unsupported data codec: %v", resp2.DataCodec)
	}

	diff := int(subtractResp.Result)
	if diff != 30 {
		t.Errorf("expected 30, got %d", diff)
	}
	t.Logf("✅ Typed RPC call successful: 50 - 20 = %d", diff)
}
