# 重试机制

## 概述

重试拦截器对可重试的错误自动重发请求，提升调用成功率，对调用方透明。通常配置在**客户端侧**，对幂等操作有效。

**源码位置**：`pkg/interceptor/retry.go`（68 行）

## 实现

```go
func NewRetryInterceptor(maxRetries int, interval time.Duration) interceptor.Interceptor {
    return func(ctx context.Context, req *protocol.Request,
        next interceptor.Invoker) (interface{}, error) {

        var lastErr error
        for attempt := 0; attempt <= maxRetries; attempt++ {
            if attempt > 0 {
                // 等待重试间隔，或 ctx 超时
                select {
                case <-time.After(interval):
                case <-ctx.Done():
                    return nil, ctx.Err()
                }
            }

            result, err := next(ctx, req)
            if err == nil {
                return result, nil  // 成功，直接返回
            }

            if !isRetryable(err) {
                return nil, err     // 不可重试，立即返回
            }
            lastErr = err
            // 继续下一次重试
        }
        return nil, lastErr // 所有重试耗尽
    }
}
```

## 可重试错误码

```go
// pkg/interceptor/retry.go
func isRetryable(err error) bool {
    var protoErr *protocol.Error
    if !errors.As(err, &protoErr) {
        // 非协议错误（如网络错误）也允许重试
        return errors.Is(err, io.EOF) ||
               errors.Is(err, io.ErrUnexpectedEOF) ||
               errors.Is(err, syscall.ECONNRESET)
    }

    switch protoErr.Code {
    case protocol.Unavailable:       // 14：服务暂时不可用（熔断）✅
        return true
    case protocol.DeadlineExceeded:  // 4：超时                    ✅
        return true
    case protocol.ResourceExhausted: // 8：被限流                  ✅
        return true
    default:
        return false
    }
}
```

## 不可重试错误码

| 错误码 | 原因 |
|--------|------|
| `InvalidArgument(3)` | 参数非法，重试无意义 |
| `NotFound(5)` | 方法不存在，重试无意义 |
| `AlreadyExists(6)` | 资源已存在，重试可能创建重复 |
| `PermissionDenied(7)` | 权限不足，重试无意义 |
| `Internal(13)` | 服务端内部错误，可能有副作用 |
| `Canceled(1)` | 客户端主动取消，不应重试 |

## 配置

```go
// 客户端：最多重试 3 次，间隔 100ms
cli, _ := client.NewClient("127.0.0.1:8080",
    client.WithInterceptors(
        interceptor.NewRetryInterceptor(3, 100*time.Millisecond),
    ),
)
```

**参数说明**：
- `maxRetries = 3`：除首次尝试外，最多再重试 3 次，共 4 次尝试
- `interval = 100ms`：固定间隔重试（可改造为指数退避）

## 重要警告：幂等性

**重试只适合幂等操作**（重复执行结果相同）：

```go
// ✅ 幂等：查询操作
cli.Call(ctx, "UserService", "GetUser", req)
cli.Call(ctx, "UserService", "ListUsers", req)

// ⚠️ 需要服务端幂等保证
cli.Call(ctx, "OrderService", "CreateOrder", req) // 可能重复创建！
cli.Call(ctx, "PaymentService", "Deduct", req)    // 可能重复扣款！
```

对于非幂等操作（Create/Update/Delete），有两种方案：

1. **不配置重试**：对该服务/方法不使用重试拦截器
2. **服务端幂等键**：客户端生成 `RequestID`，服务端检查幂等性：

```go
// 客户端生成幂等键
idempotencyKey := uuid.New().String()
req.Metadata.Set("idempotency-key", idempotencyKey)

// 服务端检查
func (s *OrderService) CreateOrder(ctx context.Context,
    req *OrderRequest) (*OrderResponse, error) {

    rpcReq := ctx.Value("rpc-request").(*protocol.Request)
    key := rpcReq.Metadata.Get("idempotency-key")

    // 检查 key 是否已处理过
    if resp, ok := idempotencyStore.Get(key); ok {
        return resp, nil // 返回缓存的响应
    }
    // 正常创建...
    idempotencyStore.Set(key, resp, 24*time.Hour)
    return resp, nil
}
```

## 指数退避（扩展）

当前实现使用固定间隔。对于容量有限的服务（频繁限流），指数退避更友好：

```go
// 自定义指数退避重试
func NewExponentialRetryInterceptor(maxRetries int,
    baseDelay, maxDelay time.Duration) interceptor.Interceptor {

    return func(ctx context.Context, req *protocol.Request,
        next interceptor.Invoker) (interface{}, error) {

        delay := baseDelay
        for attempt := 0; attempt <= maxRetries; attempt++ {
            if attempt > 0 {
                jitter := time.Duration(rand.Int63n(int64(delay) / 2))
                select {
                case <-time.After(delay + jitter): // 加随机抖动避免惊群
                case <-ctx.Done():
                    return nil, ctx.Err()
                }
                delay = min(delay*2, maxDelay) // 指数增长，上限 maxDelay
            }
            result, err := next(ctx, req)
            if err == nil || !isRetryable(err) {
                return result, err
            }
        }
        return nil, ErrMaxRetriesExceeded
    }
}
```

## 重试与其他机制的交互

```
重试 × 熔断：
  - 重试会增加对服务的请求量（放大效应）
  - 多次重试失败会加速触发熔断
  - 建议：重试次数 ≤ 3，间隔 ≥ 100ms

重试 × 限流：
  - 限流触发（ResourceExhausted）→ 可重试
  - 重试前等待间隔，给限流器时间恢复令牌
  - 建议：间隔 ≥ 1s / Rate（令牌补充时间）

重试 × 超时：
  - ctx 超时后中断重试（`case <-ctx.Done()`）
  - 确保总重试时间在客户端超时内
  - 建议：callTimeout > (maxRetries + 1) × (singleCallTime + interval)
```

## 相关文档

- [拦截器链](../server/interceptors.md)
- [熔断器](circuit-breaker.md)
- [限流器](rate-limiter.md)
- [错误码](../protocol/error-codes.md) — 可重试错误码
