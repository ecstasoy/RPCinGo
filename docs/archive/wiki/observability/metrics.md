# Prometheus 指标

## 概述

RPCinGo 通过 Metrics 拦截器向 Prometheus 上报 RPC 调用指标，使用 `github.com/prometheus/client_golang v1.23.2`。

**源码位置**：`pkg/interceptor/metrics.go`（57 行）

## 内置指标

### rpc_calls_total（Counter）

每次 RPC 调用结束后递增，按服务、方法、状态分组：

```go
rpc_calls_total{service="UserService", method="GetUser", status="success"} 15234
rpc_calls_total{service="UserService", method="GetUser", status="error"}   142
rpc_calls_total{service="UserService", method="ListUsers", status="success"} 3421
```

**标签（Labels）**：

| 标签 | 含义 | 示例值 |
|------|------|--------|
| `service` | 服务名 | `"UserService"` |
| `method` | 方法名 | `"GetUser"` |
| `status` | 调用结果 | `"success"` / `"error"` |

### rpc_duration_seconds（Histogram）

记录每次调用的延迟分布，按服务、方法分组：

```go
rpc_duration_seconds_bucket{service="UserService", method="GetUser", le="0.005"} 8234
rpc_duration_seconds_bucket{service="UserService", method="GetUser", le="0.01"}  12100
rpc_duration_seconds_bucket{service="UserService", method="GetUser", le="0.025"} 14900
rpc_duration_seconds_bucket{service="UserService", method="GetUser", le="0.05"}  15100
rpc_duration_seconds_bucket{service="UserService", method="GetUser", le="+Inf"}  15234
rpc_duration_seconds_sum{service="UserService", method="GetUser"}   47.23
rpc_duration_seconds_count{service="UserService", method="GetUser"} 15234
```

默认桶边界（`prometheus.DefBuckets`）：

```
0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10（秒）
```

## 指标注册

指标在包的 `init()` 中自动注册到默认 Prometheus 注册表：

```go
var (
    rpcCallsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "rpc_calls_total",
            Help: "Total number of RPC calls",
        },
        []string{"service", "method", "status"},
    )

    rpcDurationSeconds = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "rpc_duration_seconds",
            Help:    "RPC call duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"service", "method"},
    )
)

func init() {
    prometheus.MustRegister(rpcCallsTotal, rpcDurationSeconds)
}
```

## 暴露指标 Endpoint

在服务端启动时添加 Prometheus HTTP handler：

```go
import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // RPC 服务
    srv := server.NewServer(
        server.WithAddress(":8080"),
        server.WithInterceptors(
            interceptor.NewMetricsInterceptor(),
        ),
    )
    go srv.Start(ctx)

    // Prometheus 指标端点
    http.Handle("/metrics", promhttp.Handler())
    go http.ListenAndServe(":9090", nil)
}
```

访问 `http://localhost:9090/metrics` 查看指标。

## Prometheus 配置

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'rpcing'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

## 关键 PromQL 查询

### QPS（每秒请求数）

```promql
# 整体 QPS
rate(rpc_calls_total[1m])

# 按服务分组的 QPS
sum by (service) (rate(rpc_calls_total[1m]))

# 特定方法的 QPS
rate(rpc_calls_total{service="UserService", method="GetUser"}[1m])
```

### 错误率

```promql
# 整体错误率
sum(rate(rpc_calls_total{status="error"}[5m])) /
sum(rate(rpc_calls_total[5m]))

# 按方法的错误率
sum by (service, method) (rate(rpc_calls_total{status="error"}[5m])) /
sum by (service, method) (rate(rpc_calls_total[5m]))
```

### 延迟 P99

```promql
# 整体 P99 延迟
histogram_quantile(0.99, sum by (le) (
    rate(rpc_duration_seconds_bucket[5m])
))

# 按方法的 P99 延迟
histogram_quantile(0.99, sum by (service, method, le) (
    rate(rpc_duration_seconds_bucket[5m])
))
```

### 平均延迟

```promql
sum by (service, method) (rate(rpc_duration_seconds_sum[5m])) /
sum by (service, method) (rate(rpc_duration_seconds_count[5m]))
```

## 连接池统计（扩展指标）

连接池统计可通过 `PoolManager.Stats()` 获取，并手动上报到 Prometheus：

```go
// 自定义 Collector 上报连接池指标
type PoolMetricsCollector struct {
    poolManager *pool.PoolManager
    activeConns *prometheus.GaugeVec
    poolHitRate *prometheus.GaugeVec
}

func (c *PoolMetricsCollector) Collect(ch chan<- prometheus.Metric) {
    for addr, stats := range c.poolManager.Stats() {
        hitRate := float64(stats.GetCount-stats.CreateCount) / math.Max(float64(stats.GetCount), 1)
        ch <- c.poolHitRate.With(prometheus.Labels{"address": addr}).Set(hitRate)
    }
}
```

## Grafana 面板建议

建议创建如下面板：

| 面板 | 类型 | PromQL |
|------|------|--------|
| 总 QPS | Graph | `sum(rate(rpc_calls_total[1m]))` |
| 错误率 | Graph | 错误 / 总计 |
| P50/P95/P99 延迟 | Graph | `histogram_quantile(0.99, ...)` |
| 各方法 QPS 热力图 | Heatmap | 按 service/method 分组 |
| 错误数 TOP 10 | Table | 按 method 排序 |

## 分布式追踪（OpenTelemetry）

RPCinGo 集成 OpenTelemetry + Jaeger，通过 `pkg/tracing/` 包和两个拦截器实现端到端追踪。

### 快速启用

```go
// 1. 初始化 TracerProvider（服务端和客户端各自调用）
shutdown, _ := tracing.InitTracerProvider("http://localhost:14268/api/traces", "my-service")
defer shutdown(context.Background())

// 2. 服务端加入 TracingServer 拦截器（必须在 Logging 之前）
server.WithInterceptors(
    interceptor.TracingServer(),
    interceptor.Logging(nil),
    interceptor.Metrics(),
)

// 3. 客户端加入 TracingClient 拦截器
cli.Use(interceptor.TracingClient())
```

### 日志与追踪联动

`Logging` 拦截器自动从 context 中的 OTel span 提取 TraceID：

```
[INFO] → RPC call: [Calculator.Add] trace=b89f5c430b9f04495a2076a735be2a1b
[INFO] ✓ RPC call: [Calculator.Add] trace=b89f5c430b9f04495a2076a735be2a1b succeeded in 279µs
```

相同 TraceID 在客户端和服务端日志中同时出现，可直接 grep 追踪完整请求链路。

### Jaeger

```bash
docker run -p 14268:14268 -p 16686:16686 jaegertracing/all-in-one
```

访问 `http://localhost:16686` 查看 trace 调用树。

详见：[可观测性指南](../../deepwiki/guides/telemetry.md)

## 相关文档

- [拦截器链](../server/interceptors.md) — Metrics / Tracing 拦截器代码
- [熔断器](../reliability/circuit-breaker.md) — 熔断状态监控
