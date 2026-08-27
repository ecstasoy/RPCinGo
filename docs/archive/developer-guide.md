# RPCinGo 开发者指南

本文档面向想要使用 RPCinGo 构建后端微服务的开发者，从零开始介绍如何定义服务、部署上线、接入服务治理组件。

---

## 目录

1. [快速开始](#1-快速开始)
2. [定义服务](#2-定义服务)
3. [服务端配置](#3-服务端配置)
4. [客户端配置](#4-客户端配置)
5. [拦截器链](#5-拦截器链)
6. [服务发现与负载均衡](#6-服务发现与负载均衡)
7. [容错：熔断与限流](#7-容错熔断与限流)
8. [可观测性](#8-可观测性)
9. [YAML 配置文件](#9-yaml-配置文件)
10. [服务同时作为客户端](#10-服务同时作为客户端)
11. [生产部署清单](#11-生产部署清单)

---

## 1. 快速开始

### 1.1 安装

```bash
go get github.com/your-org/RPCinGo
```

### 1.2 定义 Protobuf 消息

```protobuf
// proto/user/user.proto
syntax = "proto3";
package user;
option go_package = "yourproject/proto/user";

message CreateUserRequest {
  string name  = 1;
  string email = 2;
}

message CreateUserResponse {
  string id = 1;
}

message GetUserRequest {
  string id = 1;
}

message GetUserResponse {
  string id    = 1;
  string name  = 2;
  string email = 3;
}
```

```bash
protoc --go_out=. --go_opt=paths=source_relative proto/user/user.proto
```

### 1.3 实现服务端

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "yourproject/proto/user"
    "RPCinGo/pkg/interceptor"
    "RPCinGo/pkg/protocol"
    "RPCinGo/pkg/server"
)

// 1. 定义服务结构体
type UserService struct {
    // 可注入数据库连接、缓存等依赖
}

// 2. 方法签名：func(ctx, *ProtoRequest) (*ProtoResponse, error)
func (s *UserService) CreateUser(ctx context.Context, req *user.CreateUserRequest) (*user.CreateUserResponse, error) {
    // 业务逻辑
    id := generateID()
    return &user.CreateUserResponse{Id: id}, nil
}

func (s *UserService) GetUser(ctx context.Context, req *user.GetUserRequest) (*user.GetUserResponse, error) {
    // 查询数据库
    return &user.GetUserResponse{
        Id:    req.Id,
        Name:  "Alice",
        Email: "alice@example.com",
    }, nil
}

func main() {
    // 3. 创建 Server
    srv := server.NewServer(
        server.WithAddress(":8080"),
        server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip),
        server.WithInterceptors(
            interceptor.Recovery(),
            interceptor.Logging(nil),
            interceptor.Metrics(),
        ),
    )

    // 4. 注册服务（自动通过反射发现所有 public 方法）
    if err := srv.RegisterService("UserService", &UserService{}); err != nil {
        fmt.Println(err)
        os.Exit(1)
    }

    // 5. 启动
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        if err := srv.Start(ctx); err != nil {
            fmt.Printf("server error: %v\n", err)
        }
    }()

    <-sigCh
    srv.Stop()
}
```

### 1.4 实现客户端

```go
package main

import (
    "context"
    "fmt"
    "time"

    "yourproject/proto/user"
    "RPCinGo/pkg/client"
    "RPCinGo/pkg/interceptor"
)

func main() {
    cli, err := client.NewClient("127.0.0.1:8080")
    if err != nil {
        panic(err)
    }
    cli.Use(interceptor.Logging(nil))
    defer cli.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // CallTyped: 类型安全的调用方式
    req := &user.CreateUserRequest{Name: "Alice", Email: "alice@example.com"}
    resp := &user.CreateUserResponse{}
    rpcResp, err := cli.CallTyped(ctx, "UserService", "CreateUser", req, resp)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Created user: %s\n", resp.Id)

    // rpcResp 可以读取 Response 级别的元数据
    if spanID, ok := rpcResp.GetMetadata("span-id"); ok {
        fmt.Printf("Server SpanID: %s\n", spanID)
    }
}
```

---

## 2. 定义服务

### 2.1 Handler 签名

RPCinGo 通过反射自动注册服务结构体的 public 方法，支持三种签名：

```go
// 类型1：Typed（推荐）— 入参和返回值都是 proto.Message
func (s *MyService) MethodA(ctx context.Context, req *pb.SomeRequest) (*pb.SomeResponse, error)

// 类型2：Context + 通用参数
func (s *MyService) MethodB(ctx context.Context, args interface{}) (interface{}, error)

// 类型3：无 Context
func (s *MyService) MethodC(args interface{}) (interface{}, error)
```

推荐使用类型 1，配合 Protobuf 可获得类型安全和 `CallTyped` 支持。

### 2.2 手动注册单个方法

如果不想用反射，可以手动注册：

```go
srv.RegisterMethod("UserService", "GetUser", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
    var getReq user.GetUserRequest
    if err := proto.Unmarshal(req.Args.([]byte), &getReq); err != nil {
        return nil, err
    }
    // 业务逻辑...
    return &user.GetUserResponse{Id: getReq.Id, Name: "Alice"}, nil
})
```

### 2.3 调用方式

| 方式 | 方法 | 适用场景 |
|------|------|---------|
| 类型安全 | `cli.CallTyped(ctx, service, method, reqPB, respPB)` | Protobuf 消息，推荐 |
| 通用 | `cli.Call(ctx, service, method, args)` | JSON 或动态参数 |

`CallTyped` 返回 `(*protocol.Response, error)`，可通过 `Response.GetMetadata()` 读取服务端写回的元数据（如 SpanID）。

---

## 3. 服务端配置

### 3.1 Option 一览

```go
srv := server.NewServer(
    // 基础
    server.WithAddress(":8080"),                                          // 监听地址
    server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip), // 编解码 + 压缩
    server.WithTimeout(30*time.Second, 30*time.Second),                   // 读/写超时

    // 并发控制
    server.WithConcurrency(10000, 16),  // 最大并发请求数, Worker Pool 大小

    // 拦截器
    server.WithInterceptors(
        interceptor.TracingServer(),
        interceptor.Recovery(),
        interceptor.Logging(nil),
        interceptor.Metrics(),
    ),

    // 限流（会自动 prepend 到拦截器链最前面）
    server.WithRateLimit(ratelimiter.NewTokenBucketLimiter(1000, 2000)),

    // 服务注册
    server.WithRegistry("UserService", "v1.0", etcdRegistry),
    server.WithHeartbeatInterval(5 * time.Second),
)
```

### 3.2 默认值

| 配置 | 默认值 |
|------|--------|
| 地址 | `:8080` |
| 编码 | JSON |
| 压缩 | 无 |
| 读写超时 | 10s |
| 最大并发 | 无限制 |
| Worker Pool | 8 |
| 心跳间隔 | 5s |

---

## 4. 客户端配置

### 4.1 Fixed 模式（直连）

适用于测试或已知服务地址的场景：

```go
cli, err := client.NewClient("127.0.0.1:8080",
    client.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip),
    client.WithTimeout(5 * time.Second),
    client.WithPoolSize(50, 5),                              // 连接池最大50，最小5
    client.WithRetry(3, 100*time.Millisecond),               // 重试3次，间隔100ms
    client.WithClientInterceptors(interceptor.Logging(nil)), // 自定义拦截器
)
```

### 4.2 Discovery 模式（服务发现）

适用于微服务架构：

```go
cli, err := client.NewDiscoveryClient(
    client.WithDiscovery(etcdDiscovery),
    client.WithLoadBalancer(loadbalancer.NewConsistentHash()),
    client.WithWatch(true),           // 监听实例变更
    client.WithCircuitBreaker(true),  // 启用熔断
    client.WithRetry(3, 200*time.Millisecond),
)
```

### 4.3 默认值

| 配置 | 默认值 |
|------|--------|
| 编码 | JSON |
| 最大连接数 | 100 |
| 最小连接数 | 10 |
| 空闲超时 | 90s |
| 调用超时 | 5s |
| 负载均衡 | RoundRobin |
| Watch | 开启 |
| 熔断器 | 开启 |

---

## 5. 拦截器链

### 5.1 执行模型

拦截器按洋葱模型执行，注册顺序 = 外到内：

```
请求 → TracingServer → Recovery → Logging → Metrics → Handler
响应 ← TracingServer ← Recovery ← Logging ← Metrics ← Handler
```

### 5.2 内置拦截器

| 拦截器 | 服务端/客户端 | 作用 |
|--------|------------|------|
| `Recovery()` | 服务端 | panic 恢复 + 堆栈捕获 |
| `Logging(logger)` | 双侧 | 日志（自动附加 TraceID） |
| `Metrics()` | 服务端 | Prometheus 指标上报 |
| `TracingServer()` | 服务端 | 提取 trace context，创建子 span |
| `TracingClient()` | 客户端 | 创建 span，注入 trace context |
| `Retry(n, interval)` | 客户端 | 基础设施错误自动重试 |
| `RateLimit(limiter)` | 双侧 | 令牌桶/滑动窗口限流 |

### 5.3 自定义拦截器

```go
func MyInterceptor() interceptor.Interceptor {
    return func(ctx context.Context, req *protocol.Request, invoker interceptor.Invoker) (any, error) {
        // --- 前置逻辑 ---
        userID, _ := req.GetMetadata("user-id")
        fmt.Printf("user %s calling %s.%s\n", userID, req.Service, req.Method)

        // 调用下一层
        result, err := invoker(ctx, req)

        // --- 后置逻辑 ---
        if err != nil {
            // 报警、审计等
        }
        return result, err
    }
}

// 注册
srv.Use(MyInterceptor())
// 或
cli.Use(MyInterceptor())
```

### 5.4 推荐的拦截器顺序

**服务端：**
```go
server.WithInterceptors(
    interceptor.TracingServer(),  // 最外层：建立 span，让后续拦截器都能拿到 TraceID
    interceptor.Recovery(),       // 兜底 panic
    interceptor.Logging(nil),     // 记录日志（含 TraceID）
    interceptor.Metrics(),        // 最内层：只统计 Handler 本身的耗时
)
```

**客户端：**
```go
cli.Use(
    interceptor.TracingClient(),  // 创建 span
    interceptor.Logging(nil),     // 记录日志
)
// WithRetry 和 WithRateLimit 会自动 prepend 到最外层
```

---

## 6. 服务发现与负载均衡

### 6.1 etcd 服务注册

**服务端注册：**

```go
import "RPCinGo/pkg/registry/etcd"

reg, err := etcd.NewEtcdRegistry(&etcd.Config{
    Endpoints:   []string{"localhost:2379"},
    DialTimeout: 5 * time.Second,
    KeyPrefix:   "/rpc/services",
    LeaseTTL:    10, // 10 秒租约，自动续约
})

srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithRegistry("UserService", "v1.0", reg),
    server.WithHeartbeatInterval(5 * time.Second),
)
```

服务启动后自动：
1. 创建 etcd Lease（10s TTL）
2. 注册 key：`/rpc/services/UserService/<instance-id>`
3. 后台 KeepAlive 续约
4. `srv.Stop()` 时自动注销

**客户端发现：**

```go
disc, err := etcd.NewEtcdDiscovery(&etcd.Config{
    Endpoints:   []string{"localhost:2379"},
    DialTimeout: 5 * time.Second,
    KeyPrefix:   "/rpc/services",
})

cli, err := client.NewDiscoveryClient(
    client.WithDiscovery(disc),
    client.WithWatch(true), // Watch 实时监听实例变更
)
```

### 6.2 内存注册中心（测试用）

```go
import "RPCinGo/pkg/registry/memory"

reg := memory.NewMemoryRegistry()
```

### 6.3 负载均衡策略

```go
import "RPCinGo/pkg/loadbalancer"

// 轮询（默认）
client.WithLoadBalancer(loadbalancer.NewRoundRobin())

// 随机
client.WithLoadBalancer(loadbalancer.NewRandom())

// 加权轮询（按 instance.Weight 分配）
client.WithLoadBalancer(loadbalancer.NewWeightedRoundRobin())

// 一致性哈希（相同 key 路由到同一实例，适合有状态场景）
client.WithLoadBalancer(loadbalancer.NewConsistentHash())
```

---

## 7. 容错：熔断与限流

### 7.1 熔断器

客户端通过 `WithCircuitBreaker(true)` 开启，按服务粒度自动创建，默认配置：

```go
circuitbreaker.DefaultConfig()
// MinRequests:      5      — 至少 5 个请求才会统计
// FailureThreshold: 0.5    — 错误率 > 50% 触发熔断
// Timeout:          60s    — 熔断后 60s 进入 HalfOpen
// SuccessThreshold: 2      — HalfOpen 连续成功 2 次恢复
// Interval:         10s    — 滑动窗口 10s（10 个桶）
```

状态流转：
```
Closed → (错误率超阈值) → Open → (60s 后) → HalfOpen → (连续成功) → Closed
                                              ↓ (失败)
                                             Open
```

### 7.2 限流器

**令牌桶（推荐用于 API 限流）：**

```go
import "RPCinGo/pkg/ratelimiter"

limiter := ratelimiter.NewTokenBucketLimiter(
    1000,  // 每秒生成 1000 个令牌
    2000,  // 桶容量 2000（允许突发）
)

// 服务端限流
srv := server.NewServer(
    server.WithRateLimit(limiter),
)

// 或客户端限流
cli, _ := client.NewClient("...",
    client.WithRateLimit(limiter),
)
```

**滑动窗口（适合精确控制窗口内总量）：**

```go
limiter := ratelimiter.NewSlidingWindowLimiter(
    100,           // 窗口内最多 100 个请求
    time.Minute,   // 窗口大小 1 分钟
)
```

### 7.3 重试

```go
cli, _ := client.NewClient("...",
    client.WithRetry(3, 200*time.Millisecond),
)
```

只对基础设施错误重试：
- `Unavailable` — 服务不可用
- `DeadlineExceeded` — 超时
- `ResourceExhausted` — 限流

**不会重试**业务错误（NotFound、InvalidArgument、PermissionDenied 等）。

---

## 8. 可观测性

### 8.1 Prometheus 指标

添加 `interceptor.Metrics()` 后自动上报两个指标：

```promql
# 调用计数（按 service、method、status 分组）
rpc_calls_total{service="UserService", method="GetUser", status="success"}

# 延迟直方图
rpc_duration_seconds_bucket{service="UserService", method="GetUser", le="0.005"}
```

暴露 Endpoint：

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

go func() {
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9091", nil)
}()
```

常用 PromQL：

```promql
# QPS
sum by (service, method) (rate(rpc_calls_total[1m]))

# 错误率
sum(rate(rpc_calls_total{status="error"}[5m])) / sum(rate(rpc_calls_total[5m]))

# P99 延迟
histogram_quantile(0.99, sum by (le) (rate(rpc_duration_seconds_bucket[5m])))
```

### 8.2 分布式追踪（OpenTelemetry + Jaeger）

```go
import "RPCinGo/pkg/tracing"

// 初始化（服务端和客户端各调一次）
shutdown, err := tracing.InitTracerProvider(
    "http://localhost:14268/api/traces",  // Jaeger 地址
    "user-service",                        // 服务名
)
defer shutdown(context.Background())

// 服务端
server.WithInterceptors(
    interceptor.TracingServer(),  // 提取上游 trace context，创建子 span
    interceptor.Logging(nil),     // 自动注入 trace=<id> 到日志
)

// 客户端
cli.Use(
    interceptor.TracingClient(),  // 创建 span，注入 W3C traceparent 到请求
)
```

Trace context 通过请求元数据（`req.Metadata`）传播，使用 W3C TraceContext + B3 标准。

启动 Jaeger：
```bash
docker run -p 14268:14268 -p 16686:16686 jaegertracing/all-in-one
```

访问 `http://localhost:16686` 查看调用链。

### 8.3 日志输出

`Logging` 拦截器自动从 context 提取 TraceID：

```
[INFO] → RPC call: [UserService.GetUser] trace=4bf92f3577b34da6a3ce929d0e0e4736
[INFO] ✓ RPC call: [UserService.GetUser] trace=4bf92f3577b34da6a3ce929d0e0e4736 succeeded in 1.2ms
```

自定义 Logger：

```go
type MyLogger struct{}

func (l *MyLogger) Infof(format string, args ...interface{})  { /* 接入你的日志框架 */ }
func (l *MyLogger) Errorf(format string, args ...interface{}) { /* ... */ }

interceptor.Logging(&MyLogger{})
```

---

## 9. YAML 配置文件

RPCinGo 支持从 YAML 文件加载配置，避免硬编码：

```yaml
# config.yaml
server:
  address: ":8080"
  codec: "protobuf"
  compress: "gzip"
  read_timeout: 30s
  write_timeout: 30s
  max_concurrent: 10000
  worker_pool_size: 16
  enable_registry: true
  heartbeat_interval: 5s

client:
  address: "localhost:8080"
  timeout: 5s
  codec: "protobuf"
  compress: "gzip"
  max_connections: 100
  min_connections: 10
  idle_timeout: 90s
  call_timeout: 5s
  load_balancer: "round_robin"
  watch: true
  circuit_breaker: true

registry:
  type: "etcd"
  etcd:
    endpoints: ["localhost:2379"]
    dial_timeout: 5s
    key_prefix: "/rpc/services"
    lease_ttl: 10
```

加载并使用：

```go
import "RPCinGo/pkg/config"

cfg, err := config.Load("config.yaml")
if err != nil {
    panic(err)
}

// 自动转换为 Option 切片
serverOpts := config.BuildServerOptions(cfg)
clientOpts := config.BuildClientOptions(cfg)

srv := server.NewServer(serverOpts...)
cli, _ := client.NewClient(cfg.Client.Address, clientOpts...)
```

---

## 10. 服务同时作为客户端

一个 Go 进程可以同时运行 Server 和 Client，这是微服务的标准模式：

```go
func main() {
    // 1. 本服务作为服务端
    srv := server.NewServer(
        server.WithAddress(":8080"),
        server.WithRegistry("OrderService", "v1.0", etcdRegistry),
    )
    srv.RegisterService("OrderService", &OrderService{
        inventoryCli: inventoryCli,  // 注入下游客户端
        paymentCli:   paymentCli,
    })
    go srv.Start(ctx)

    // 2. 本服务作为客户端调用下游
    inventoryCli, _ := client.NewDiscoveryClient(
        client.WithDiscovery(etcdDiscovery),
    )
    paymentCli, _ := client.NewDiscoveryClient(
        client.WithDiscovery(etcdDiscovery),
    )
}

// Handler 中调用下游时传递 ctx，保证 TraceID 串联
func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderReq) (*pb.CreateOrderResp, error) {
    // ctx 中已有上游传来的 span
    // TracingClient 拦截器会自动以此为 parent 创建子 span
    stockResp := &pb.CheckStockResp{}
    _, err := s.inventoryCli.CallTyped(ctx, "InventoryService", "CheckStock", stockReq, stockResp)
    if err != nil {
        return nil, err
    }
    // ...
}
```

Jaeger 中看到的调用链：

```
Gateway → OrderService.CreateOrder → InventoryService.CheckStock
                                   → PaymentService.Charge
```

---

## 11. 生产部署清单

### 服务端

```go
srv := server.NewServer(
    // 基础
    server.WithAddress(":8080"),
    server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip),
    server.WithTimeout(30*time.Second, 30*time.Second),
    server.WithConcurrency(10000, 32),

    // 限流
    server.WithRateLimit(ratelimiter.NewTokenBucketLimiter(5000, 10000)),

    // 拦截器（顺序重要）
    server.WithInterceptors(
        interceptor.TracingServer(),
        interceptor.Recovery(),
        interceptor.Logging(nil),
        interceptor.Metrics(),
    ),

    // 服务注册
    server.WithRegistry("UserService", "v1.0", etcdRegistry),
    server.WithHeartbeatInterval(5*time.Second),
)
```

### 客户端

```go
cli, _ := client.NewDiscoveryClient(
    client.WithDiscovery(etcdDiscovery),
    client.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip),
    client.WithTimeout(3*time.Second),
    client.WithPoolSize(50, 5),
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
    client.WithCircuitBreaker(true),
    client.WithRetry(3, 200*time.Millisecond),
    client.WithRateLimit(ratelimiter.NewTokenBucketLimiter(1000, 2000)),
    client.WithClientInterceptors(
        interceptor.TracingClient(),
        interceptor.Logging(nil),
    ),
)
```

### 可观测性基础设施

```bash
# Jaeger
docker run -p 14268:14268 -p 16686:16686 jaegertracing/all-in-one

# Prometheus + Grafana（docker-compose.yml）
services:
  prometheus:
    image: prom/prometheus
    ports: ["9090:9090"]
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
  grafana:
    image: grafana/grafana
    ports: ["3000:3000"]
```

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'rpcing'
    static_configs:
      - targets: ['host.docker.internal:9091']
```

### Metrics Endpoint

```go
go func() {
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9091", nil)
}()
```

---

## 附录：元数据传递

请求和响应都支持元数据，可用于传递认证信息、追踪 ID 等：

```go
// 客户端设置
req := protocol.NewRequest("UserService", "GetUser", args)
req.SetMetadata("auth-token", "Bearer xxx")
req.SetMetadata("user-id", "12345")

// 服务端读取（在 Handler 或自定义拦截器中）
func MyAuthInterceptor() interceptor.Interceptor {
    return func(ctx context.Context, req *protocol.Request, invoker interceptor.Invoker) (any, error) {
        token, ok := req.GetMetadata("auth-token")
        if !ok || !validateToken(token) {
            return nil, fmt.Errorf("unauthorized")
        }
        return invoker(ctx, req)
    }
}
```

内置元数据 key：

| Key | 用途 |
|-----|------|
| `trace-id` | 分布式追踪 Trace ID |
| `span-id` | 分布式追踪 Span ID |
| `auth-token` | 认证令牌 |
| `user-id` | 用户标识 |
| `region` / `zone` | 地域/可用区 |
| `debug` | 调试标志 |
