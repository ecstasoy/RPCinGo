# 可观测性指南（Telemetry）

## 概述

RPCinGo 通过 `Metrics` 拦截器集成 Prometheus，提供开箱即用的 RPC 调用指标。同时，`Logging` 拦截器输出结构化日志，支持分布式追踪（trace-id）。

## Prometheus 指标

### 注册指标

| 指标名 | 类型 | Labels | 说明 |
|--------|------|--------|------|
| `rpc_calls_total` | Counter | `service`, `method`, `status` | RPC 调用总次数（按服务/方法/状态分组） |
| `rpc_duration_seconds` | Histogram | `service`, `method` | RPC 调用耗时分布 |

### Histogram Buckets

默认桶边界（单位：秒）：

```
0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0
```

覆盖从 5ms 到 10s 的延迟范围，适合微服务 RPC 场景。

### 常用 PromQL 查询

| 查询目的 | PromQL |
|---------|--------|
| QPS（每秒调用数） | `rate(rpc_calls_total[1m])` |
| 错误率 | `rate(rpc_calls_total{status!="OK"}[1m]) / rate(rpc_calls_total[1m])` |
| P99 延迟 | `histogram_quantile(0.99, rate(rpc_duration_seconds_bucket[5m]))` |
| P50 延迟 | `histogram_quantile(0.50, rate(rpc_duration_seconds_bucket[5m]))` |
| 各服务 QPS 对比 | `sum by(service) (rate(rpc_calls_total[1m]))` |
| 各方法错误分布 | `sum by(method, status) (rate(rpc_calls_total[1m]))` |

## 启用 Metrics 拦截器

```go
import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// 1. 添加 Metrics 拦截器
srv := server.New(
    server.WithAddress(":8080"),
    server.WithInterceptors(
        interceptor.Recovery(),
        interceptor.Logging(),
        interceptor.Metrics(),  // 启用 Prometheus 指标
    ),
)

// 2. 暴露 /metrics 端点（通常在独立 goroutine 中）
go func() {
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9090", nil)
}()
```

## 日志结构

`Logging` 拦截器输出 JSON 格式结构化日志：

```json
{
  "time": "2026-03-31T10:00:00Z",
  "level": "INFO",
  "service": "Calculator",
  "method": "Add",
  "trace_id": "abc123",
  "duration_ms": 1.234,
  "status": "OK"
}
```

错误情况下附加字段：

```json
{
  "status": "Internal",
  "error": "division by zero"
}
```

## 分布式追踪（OpenTelemetry + Jaeger）

RPCinGo 通过 `pkg/tracing/` + `interceptor.TracingClient/TracingServer` 实现完整的分布式追踪，兼容 W3C TraceContext 和 B3 传播协议。

### 初始化 TracerProvider

```go
// 服务端 main()
shutdown, err := tracing.InitTracerProvider("http://localhost:14268/api/traces", "my-service")
if err != nil {
    log.Fatal(err)
}
defer shutdown(context.Background())
```

```go
// 客户端 main()
shutdown, err := tracing.InitTracerProvider("http://localhost:14268/api/traces", "my-client")
defer shutdown(context.Background())
```

### 接入拦截器链

```go
// 服务端
srv := server.NewServer(
    server.WithInterceptors(
        interceptor.TracingServer(), // 必须第一个：从 req.Metadata 提取 trace 上下文
        interceptor.Recovery(),
        interceptor.Logging(nil),    // 自动打印 TraceID
        interceptor.Metrics(),
    ),
)

// 客户端
cli.Use(interceptor.TracingClient()) // 创建 client span，注入到 req.Metadata
```

### 追踪上下文传播原理

```
客户端                              服务端
TracingClient                       TracingServer
  │ 创建 client span                   │
  │ TraceID=T, SpanID=C               │
  │ Inject → req.Metadata             │
  │   traceparent: 00-T-C-01          │
  │   X-B3-TraceId: T                 │
  │   X-B3-SpanId: C ────────────────►│ Extract → 重建远端 SpanContext
  │                                   │ 创建 child span: TraceID=T, SpanID=S
  │                                   │ 写 req.Metadata["span-id"] = S
  │◄──────────────── resp.Metadata["span-id"] = S
```

### 读取服务端 SpanID

```go
rpcResp, err := cli.CallTyped(ctx, "Calculator", "Add", req, resp)
if spanID, ok := rpcResp.GetMetadata("span-id"); ok {
    fmt.Println("Server SpanID:", spanID)
}
```

### 启动 Jaeger

```bash
docker run -p 14268:14268 -p 16686:16686 jaegertracing/all-in-one
```

访问 `http://localhost:16686` 查看完整调用链。

### 日志与追踪联动

`Logging` 拦截器自动从 context 提取 TraceID 并输出到日志：

```
[INFO] → RPC call: [Calculator.Add] trace=b89f5c430b9f04495a2076a735be2a1b
[INFO] ✓ RPC call: [Calculator.Add] trace=b89f5c430b9f04495a2076a735be2a1b succeeded in 279µs
```

在日志管理系统（如 ELK）中用 TraceID grep，即可将日志与 Jaeger trace 关联。

## 连接池指标

`ConnectionPool.Stats()` 提供运行时连接池状态：

```go
stats := pool.Stats()
// stats.GetCount    - 总获取次数
// stats.CreateCount - 新建连接次数
// stats.CloseCount  - 关闭连接次数
// stats.CurrentSize - 当前空闲连接数
// stats.InUse       - 当前使用中连接数
```

可将这些指标定期上报到 Prometheus Gauge：

```go
prometheus.MustRegister(prometheus.NewGaugeFunc(
    prometheus.GaugeOpts{Name: "rpc_pool_idle_connections"},
    func() float64 { return float64(pool.Stats().CurrentSize) },
))
```

## 熔断器状态监控

```go
// 定期检查熔断器状态
cb := circuitbreaker.New(...)
state := cb.State()
switch state {
case circuitbreaker.StateClosed:
    // 正常
case circuitbreaker.StateOpen:
    // 告警：熔断打开
case circuitbreaker.StateHalfOpen:
    // 观察：正在恢复探测
}
```

## 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|---------|
| Metrics 拦截器 panic "already registered" | 测试中多次注册 | 在测试 setup 中使用 `prometheus.NewRegistry()` |
| `rpc_calls_total` 中无数据 | 未添加 Metrics 拦截器 | 确保 `interceptor.Metrics()` 在链中 |
| trace-id 在日志中为空 | 客户端未设置 Metadata | 在 Request.Metadata 中添加 `"trace-id"` |

## Source References

- `pkg/interceptor/metrics.go`
- `pkg/interceptor/logging.go`
- `pkg/interceptor/interceptor.go`
- `pkg/pool/pool.go`（Stats()）
- `wiki/observability/metrics.md`
