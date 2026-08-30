package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ecstasoy/RPCinGo/pkg/interceptor"
	"github.com/ecstasoy/RPCinGo/pkg/tracing"

	"github.com/ecstasoy/RPCinGo/examples/proto/calculator"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/server"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type CalculatorService struct{}

func (s *CalculatorService) Add(ctx context.Context, req *calculator.AddRequest) (*calculator.AddResponse, error) {
	result := req.A + req.B
	fmt.Printf("Server: Add(%d, %d) = %d\n", req.A, req.B, result)
	return &calculator.AddResponse{Result: result}, nil
}

func (s *CalculatorService) Subtract(ctx context.Context, req *calculator.SubtractRequest) (*calculator.SubtractResponse, error) {
	result := req.A - req.B
	fmt.Printf("Server: Subtract(%d, %d) = %d\n", req.A, req.B, result)
	return &calculator.SubtractResponse{Result: result}, nil
}

func (s *CalculatorService) Multiply(ctx context.Context, req *calculator.MultiplyRequest) (*calculator.MultiplyResponse, error) {
	result := req.A * req.B
	fmt.Printf("Server: Multiply(%d, %d) = %d\n", req.A, req.B, result)
	return &calculator.MultiplyResponse{Result: result}, nil
}

func (s *CalculatorService) Divide(ctx context.Context, req *calculator.DivideRequest) (*calculator.DivideResponse, error) {
	if req.B == 0 {
		return nil, fmt.Errorf("division by zero")
	}
	result := req.A / req.B
	remainder := req.A % req.B
	fmt.Printf("Server: Divide(%d, %d) = %d remainder %d\n", req.A, req.B, result, remainder)
	return &calculator.DivideResponse{Result: result, Remainder: remainder}, nil
}

func main() {
	shutdown, err := tracing.InitTracerProvider("http://localhost:14268/api/traces", "calculator")
	if err != nil {
		fmt.Printf("init tracer failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()

	srv := server.NewServer(
		server.WithAddress("127.0.0.1:8080"),
		server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone),
		server.WithInterceptors(
			interceptor.TracingServer(),
			interceptor.Recovery(),
			interceptor.Logging(nil),
			interceptor.Metrics(),
		),
	)

	calcService := &CalculatorService{}
	if err := srv.RegisterService("Calculator", calcService); err != nil {
		fmt.Printf("RegisterService failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Calculator Server starting on 127.0.0.1:8080...")
	fmt.Println("Registered service: Calculator")
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		_ = http.ListenAndServe(":9091", nil)
	}()

	go func() {
		if err := srv.Start(ctx); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down server...")
	_ = srv.Stop()
	fmt.Println("Server stopped")
}
