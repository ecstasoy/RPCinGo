# 拦截器链

## 概述

拦截器（Interceptor）是 RPCinGo 的横切关注点机制，类似 HTTP 中间件。多个拦截器组合成有序链，在 handler 调用前后插入逻辑。

**源码位置**：`pkg/interceptor/`

## 核心类型

**源码**：`pkg/interceptor/interceptor.go`（42 行）

```go
// 调用链的下一个节点（最终是实际的 handler）
type Invoker func(ctx context.Context, req *protocol.Request) (interface{}, error)

// 拦截器函数类型
type Interceptor func(ctx context.Context, req *protocol.Request, next Invoker) (interface{}, error)

// 链：按注册顺序逆向嵌套，形成洋葱结构
type Chain struct {
    interceptors []Interceptor
}

func NewChain(interceptors ...Interceptor) *Chain {
    return &Chain{interceptors: interceptors}
}

func (c *Chain) Execute(ctx context.Context, req *protocol.Request, final Invoker) (interface{}, error) {
    // 从后往前构建嵌套闭包
    h := final
    for i := len(c.interceptors) - 1; i >= 0; i-- {
        next := h                        // := 每次迭代创建新变量，闭包捕获的是本次迭代的副本
        interceptor := c.interceptors[i] // 同上，在所有 Go 版本中均正确（不依赖 Go 1.22 的 for 变量语义修复）
        h = func(ctx context.Context, req *protocol.Request) (interface{}, error) {
            return interceptor(ctx, req, next)
        }
    }
    return h(ctx, req)
}
```

## 执行顺序

注册顺序：`[Recovery, Logging, Metrics, RateLimit]`

```
请求进入 → Recovery → Logging → Metrics → RateLimit → [Handler]
响应返回 ← Recovery ← Logging ← Metrics ← RateLimit ← [Handler]
```

## 内置拦截器

### 1. Recovery（Panic 恢复）

**源码**：`pkg/interceptor/recovery.go`（25 行）

捕获 handler 中的 panic，转化为 Internal 错误，防止 goroutine 崩溃，保证服务稳定性。

```go
func NewRecoveryInterceptor() Interceptor {
    return func(ctx context.Context, req *protocol.Request,
        next Invoker) (result interface{}, err error) {
        defer func() {
            if r := recover(); r != nil {
                stack := make([]byte, 4096)
                stack = stack[:runtime.Stack(stack, false)]
                err = fmt.Errorf("panic recovered: %v\n%s", r, stack)
                // 注意：result 已由 named return 初始化为 nil
            }
        }()
        return next(ctx, req)
    }
}
```

**配置建议**：Recovery 必须放在拦截器链**最外层**（第一个注册），确保任何 panic 都被捕获，包括后续拦截器中的 panic。

---

### 2. Logging（请求日志）

**源码**：`pkg/interceptor/logging.go`

接受 `logger.Logger`（来自 `pkg/logger`），nil 时自动使用 `logger.New()`（slog 文本格式）。日志以 slog key-value 结构化格式输出，自动提取 OTel span 中的 TraceID：

```
time=... level=INFO  msg="rpc call ok"     service=Calculator method=Add trace=b89f5c43... duration=279µs
time=... level=ERROR msg="rpc call failed" service=Calculator method=Div trace=b89f5c43... duration=1ms error="division by zero"
```

传入自定义 Logger（如 zap 适配器）：

```go
srv.Use(interceptor.Logging(myZapLogger)) // 实现 logger.Logger 接口即可
```

**注意**：`TracingServer()` 必须在 `Logging()` 之前注册，否则 trace 字段为空字符串。

---

### 3. Metrics（Prometheus 监控）

**源码**：`pkg/interceptor/metrics.go`（57 行）

```go
var (
    // 请求计数器，标签：service, method, status(success/error)
    rpcCallsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "rpc_calls_total",
            Help: "Total number of RPC calls",
        },
        []string{"service", "method", "status"},
    )

    // 请求延迟分布，标签：service, method
    rpcDurationSeconds = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "rpc_duration_seconds",
            Help:    "RPC call duration in seconds",
            Buckets: prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
        },
        []string{"service", "method"},
    )
)

func init() {
    prometheus.MustRegister(rpcCallsTotal, rpcDurationSeconds)
}

func NewMetricsInterceptor() Interceptor {
    return func(ctx context.Context, req *protocol.Request, next Invoker) (interface{}, error) {
        start := time.Now()

        result, err := next(ctx, req)

        duration := time.Since(start).Seconds()
        status := "success"
        if err != nil {
            status = "error"
        }

        rpcCallsTotal.WithLabelValues(req.Service, req.Method, status).Inc()
        rpcDurationSeconds.WithLabelValues(req.Service, req.Method).Observe(duration)

        return result, err
    }
}
```

---

### 4. RateLimit（限流）

**源码**：`pkg/interceptor/ratelimit.go`（20 行）

```go
func NewRateLimitInterceptor(limiter ratelimiter.RateLimiter) Interceptor {
    return func(ctx context.Context, req *protocol.Request, next Invoker) (interface{}, error) {
        if !limiter.Allow() {
            return nil, ratelimiter.ErrRateLimitExceeded
            // 服务端 error_map.go 将其映射为 ResourceExhausted 错误码
        }
        return next(ctx, req)
    }
}
```

---

### 5. Retry（重试，通常用于客户端）

**源码**：`pkg/interceptor/retry.go`（68 行）

```go
func NewRetryInterceptor(maxRetries int, interval time.Duration) Interceptor {
    return func(ctx context.Context, req *protocol.Request, next Invoker) (interface{}, error) {
        var lastErr error
        for attempt := 0; attempt <= maxRetries; attempt++ {
            if attempt > 0 {
                select {
                case <-time.After(interval):
                case <-ctx.Done():
                    return nil, ctx.Err()
                }
            }

            result, err := next(ctx, req)
            if err == nil {
                return result, nil
            }
            if !isRetryable(err) {
                return nil, err
            }
            lastErr = err
        }
        return nil, lastErr
    }
}

// 可重试的错误码
func isRetryable(err error) bool {
    var protoErr *protocol.Error
    if !errors.As(err, &protoErr) {
        return false
    }
    switch protoErr.Code {
    case protocol.Unavailable,        // 服务暂时不可用（熔断）
        protocol.DeadlineExceeded,     // 超时
        protocol.ResourceExhausted:    // 被限流
        return true
    default:
        return false
    }
}
```

---

## 在 Server 中配置拦截器

```go
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithRateLimit(ratelimiter.NewTokenBucket(10000, 500)), // 便捷方式，自动前置
    server.WithInterceptors(
        interceptor.TracingServer(), // 最外层：建立 span，使 TraceID 注入 context
        interceptor.Recovery(),      // panic 兜底
        interceptor.Logging(nil),    // 日志（自动打印 TraceID）
        interceptor.Metrics(),       // Prometheus 指标
    ),
)
```

## 在 Client 中配置拦截器

```go
cli, _ := client.NewClient("127.0.0.1:8080",
    client.WithRateLimit(ratelimiter.NewTokenBucket(500, 50)), // 客户端限流
    client.WithClientInterceptors(
        interceptor.TracingClient(), // 创建 client span，注入 trace 上下文到 req.Metadata
        interceptor.Logging(nil),
    ),
)
```

## 自定义拦截器示例

```go
// 认证拦截器：验证 Metadata 中的 Token
func AuthInterceptor(tokenValidator func(token string) bool) interceptor.Interceptor {
    return func(ctx context.Context, req *protocol.Request,
        next interceptor.Invoker) (interface{}, error) {

        token := req.Metadata.Get(protocol.MetadataKeyToken)
        if token == "" {
            return nil, &protocol.Error{
                Code:    protocol.PermissionDenied,
                Message: "missing auth token",
            }
        }
        if !tokenValidator(token) {
            return nil, &protocol.Error{
                Code:    protocol.PermissionDenied,
                Message: "invalid token",
            }
        }

        // 将 userID 注入 context，供 handler 使用
        userID := req.Metadata.Get(protocol.MetadataKeyUserID)
        ctx = context.WithValue(ctx, "user-id", userID)

        return next(ctx, req)
    }
}
```

## 拦截器顺序建议

服务端标准配置（从外到内）：

```
1. TracingServer ← 最外层：建立 span、注入 TraceID 到 context（后续拦截器都能读到）
2. Recovery      ← panic 兜底
3. Logging       ← 打印 TraceID + 耗时（依赖 TracingServer 已注入的 span）
4. Metrics       ← 采集 Prometheus 指标
5. RateLimit     ← 限流（通过 WithRateLimit 选项自动前置，也可手动注册）
```

**RateLimit 位置说明**：`WithRateLimit` 选项会把限流拦截器 prepend 到整个链路最前面（比 TracingServer 还外），确保超限请求立刻拒绝，不产生任何 span 或日志开销。如果希望限流请求也被记录，可手动通过 `WithInterceptors` 把 RateLimit 放在合适位置。

## 相关文档

- [Server 概述](overview.md)
- [限流器](../reliability/rate-limiter.md)
- [重试机制](../reliability/retry.md)
- [Prometheus 指标](../observability/metrics.md)
- [Metadata](../protocol/metadata.md) — Trace ID 传递
