# RPCinGo

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](https://go.dev)

An RPC framework built from scratch in Go — custom binary protocol, TCP multiplexing, connection pooling, interceptor chains, service discovery, and full observability.

## Highlights

- **Custom Binary Protocol** — 20-byte fixed header + variable body; JSON/Protobuf codecs with Gzip compression
- **TCP Multiplexing** — concurrent requests over a single connection via RequestID dispatch
- **Onion-Model Interceptors** — pluggable middleware chain on both client and server side
- **Service Discovery** — etcd-based registration (Lease + Watch) with multiple load balancing strategies
- **Resilience** — three-state circuit breaker, token bucket rate limiter, automatic retry
- **Observability** — OpenTelemetry tracing (Jaeger), Prometheus metrics, structured logging

## Quick Start

### Server

```go
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithInterceptors(
        interceptor.Recovery(),
        interceptor.Logging(nil),
        interceptor.Metrics(),
    ),
)

// Register any struct — public methods are discovered via reflection
srv.RegisterService("Calculator", &CalculatorService{})
srv.Start(context.Background())
```

### Client

```go
cli, _ := client.NewClient("127.0.0.1:8080")
cli.Use(interceptor.TracingClient(), interceptor.Logging(nil))
defer cli.Close()

req := &pb.AddRequest{A: 10, B: 20}
resp := &pb.AddResponse{}
_, err := cli.CallTyped(ctx, "Calculator", "Add", req, resp)
fmt.Println(resp.Result) // 30
```

### With Service Discovery

```go
// Server — auto-registers to etcd with heartbeat
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithRegistry("Calculator", "v1.0", etcdRegistry),
)

// Client — discovers instances, load balances, retries on failure
cli, _ := client.NewDiscoveryClient(
    client.WithDiscovery(etcdDiscovery),
    client.WithLoadBalancer(loadbalancer.NewConsistentHash()),
    client.WithCircuitBreaker(true),
    client.WithRetry(3, 200*time.Millisecond),
)
```

## Architecture

```
┌─────────────────────────────────┐     ┌─────────────────────────────────┐
│            Client               │     │            Server               │
│                                 │     │                                 │
│  CallTyped / Call               │     │  HandleRequest                  │
│    ↓                            │     │    ↓                            │
│  Interceptor Chain              │     │  Interceptor Chain              │
│  (Tracing → Logging → Retry)    │     │  (Tracing → Recovery → Metrics) │
│    ↓                            │     │    ↓                            │
│  Connection Pool ──── TCP ──────┼─────┼→ TCP Listener                   │
│  (channel-based, health check)  │     │  (writeCh serialization)        │
│    ↓                            │     │    ↓                            │
│  Load Balancer                  │     │  Service Registry               │
│  (RR / WRR / Hash / Random)     │     │  (reflection-based dispatch)    │
│    ↓                            │     │    ↓                            │
│  Circuit Breaker (per-service)  │     │  etcd Registration              │
└─────────────────────────────────┘     └─────────────────────────────────┘
```

## Project Structure

```
pkg/
├── protocol/        # 20-byte header, Request/Response, error codes
├── codec/           # JSON/Protobuf registry, Gzip decorator
├── transport/tcp/   # Multiplexed client, graceful-shutdown server
├── pool/            # Channel-based connection pool
├── server/          # Service registry, reflection dispatch
├── client/          # Fixed/Discovery dual-mode, CallTyped API
├── interceptor/     # Recovery, Logging, Metrics, Retry, RateLimit, Tracing
├── tracing/         # OpenTelemetry + Jaeger
├── registry/        # etcd and in-memory implementations
├── loadbalancer/    # RoundRobin, Random, Weighted, ConsistentHash
├── circuitbreaker/  # Three-state breaker with sliding window
├── ratelimiter/     # Token bucket, sliding window
└── config/          # YAML config loading

examples/
├── calculator/      # Interactive calculator (add/sub/mul/div + Prometheus)
└── microservice/    # Multi-service example with etcd discovery
```

## Configuration

```yaml
server:
  address: ":8080"
  codec: "protobuf"
  compress: "gzip"
  read_timeout: 30s

client:
  timeout: 5s
  max_connections: 100
  load_balancer: "round_robin"
  circuit_breaker: true

registry:
  type: "etcd"
  etcd:
    endpoints: ["localhost:2379"]
    lease_ttl: 10
```

```go
cfg, _ := config.Load("config.yaml")
srv := server.NewServer(config.BuildServerOptions(cfg)...)
```

## Observability

**Tracing** — OpenTelemetry with Jaeger, W3C TraceContext + B3 propagation

```bash
docker run -p 14268:14268 -p 16686:16686 jaegertracing/all-in-one
```

**Metrics** — Prometheus counters and histograms

```go
// Expose /metrics endpoint
go http.ListenAndServe(":9091", promhttp.Handler())
```

## Built-in Interceptors

| Interceptor | Side | Description |
|-------------|------|-------------|
| `Recovery()` | Server | Panic recovery with stack trace |
| `Logging(logger)` | Both | Structured logs with TraceID |
| `Metrics()` | Server | Prometheus QPS + latency |
| `TracingClient()` | Client | Creates span, injects trace context |
| `TracingServer()` | Server | Extracts trace context, creates child span |
| `Retry(n, interval)` | Client | Retries on transient errors only |
| `RateLimit(limiter)` | Both | Token bucket or sliding window |

## Tech Stack

Go &middot; TCP &middot; etcd v3 &middot; Protocol Buffers &middot; OpenTelemetry &middot; Jaeger &middot; Prometheus
