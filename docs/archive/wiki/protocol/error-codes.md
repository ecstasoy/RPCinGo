# 错误码

## 概述

RPCinGo 使用结构化 `Error` 类型在协议层传递错误，服务端将 Go error 映射为协议错误码，客户端再将协议错误码还原为 Go error，双向透明传递。

**源码位置**：`pkg/protocol/error.go`、`pkg/server/error_map.go`、`pkg/client/error_map.go`

## Error 结构体

```go
// pkg/protocol/error.go
type Error struct {
    Code    ErrorCode // 错误码枚举
    Message string    // 人类可读描述
}

func (e *Error) Error() string {
    return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}
```

## 完整错误码列表

```go
type ErrorCode int32

const (
    OK                 ErrorCode = 0   // 成功
    Canceled           ErrorCode = 1   // 请求被客户端取消（context.Canceled）
    Unknown            ErrorCode = 2   // 未知错误
    InvalidArgument    ErrorCode = 3   // 参数非法（客户端错误，不应重试）
    DeadlineExceeded   ErrorCode = 4   // 超时（context.DeadlineExceeded）
    NotFound           ErrorCode = 5   // 资源不存在（服务/方法未注册）
    AlreadyExists      ErrorCode = 6   // 资源已存在
    PermissionDenied   ErrorCode = 7   // 无权限
    ResourceExhausted  ErrorCode = 8   // 资源耗尽（限流触发）
    Internal           ErrorCode = 13  // 服务端内部错误（panic、反射错误等）
    Unavailable        ErrorCode = 14  // 服务暂时不可用（熔断、过载）
)
```

> 错误码设计参考 gRPC Status Codes，数值与 gRPC 保持一致，便于未来互操作。

## 服务端错误映射

**源码**：`pkg/server/error_map.go`

`mapError` 将 Go error 转换为协议 `*Error`：

```go
func mapError(err error) *protocol.Error {
    if err == nil {
        return nil
    }
    switch {
    case errors.Is(err, ratelimiter.ErrRateLimitExceeded):
        return &protocol.Error{Code: protocol.ResourceExhausted, Message: err.Error()}

    case errors.Is(err, circuitbreaker.ErrCircuitOpen):
        return &protocol.Error{Code: protocol.Unavailable, Message: err.Error()}

    case errors.Is(err, context.Canceled):
        return &protocol.Error{Code: protocol.Canceled, Message: err.Error()}

    case errors.Is(err, context.DeadlineExceeded):
        return &protocol.Error{Code: protocol.DeadlineExceeded, Message: err.Error()}

    case errors.Is(err, ErrMethodNotFound), errors.Is(err, ErrServiceNotFound):
        return &protocol.Error{Code: protocol.NotFound, Message: err.Error()}

    default:
        return &protocol.Error{Code: protocol.Internal, Message: err.Error()}
    }
}
```

## 客户端错误还原

**源码**：`pkg/client/error_map.go`

`unmapError` 将协议 `*Error` 还原为 Go error：

```go
func unmapError(e *protocol.Error) error {
    if e == nil || e.Code == protocol.OK {
        return nil
    }
    switch e.Code {
    case protocol.Canceled:
        return context.Canceled
    case protocol.DeadlineExceeded:
        return context.DeadlineExceeded
    case protocol.NotFound:
        return fmt.Errorf("%w: %s", ErrMethodNotFound, e.Message)
    case protocol.Unavailable:
        return fmt.Errorf("%w: %s", ErrServiceUnavailable, e.Message)
    case protocol.ResourceExhausted:
        return fmt.Errorf("%w: %s", ErrRateLimitExceeded, e.Message)
    default:
        return fmt.Errorf("rpc error (code=%d): %s", e.Code, e.Message)
    }
}
```

## 错误传播完整路径

```
Handler 抛出 context.DeadlineExceeded
    │
    │ pkg/server/error_map.go mapError()
    ▼
protocol.Error{Code: 4, Message: "context deadline exceeded"}
    │
    │ Codec.Encode() → 写入 Response.Error 字段
    │ TCP 网络传输
    ▼
protocol.Error{Code: 4, Message: "context deadline exceeded"}
    │
    │ pkg/client/error_map.go unmapError()
    ▼
context.DeadlineExceeded（标准 Go error）
    │
    ▼
调用方: if errors.Is(err, context.DeadlineExceeded) { /* 超时处理 */ }
```

## 各错误码触发场景

| 错误码 | 触发场景 | 客户端应对 |
|--------|---------|-----------|
| `OK(0)` | 调用成功 | — |
| `Canceled(1)` | 客户端主动取消 ctx | 无需处理 |
| `Unknown(2)` | 未分类错误 | 记录日志，不重试 |
| `InvalidArgument(3)` | 参数校验失败 | 修复参数后重试 |
| `DeadlineExceeded(4)` | 超时 | 检查超时配置，可重试 |
| `NotFound(5)` | 服务/方法未注册 | 检查服务名拼写 |
| `AlreadyExists(6)` | 重复创建资源 | 检查业务逻辑 |
| `PermissionDenied(7)` | 权限不足 | 检查 Token |
| `ResourceExhausted(8)` | 被限流 | 降低请求速率，退避重试 |
| `Internal(13)` | 服务端 panic/内部错误 | 报告 bug，不重试 |
| `Unavailable(14)` | 熔断/服务过载 | 退避重试，检查服务健康状态 |

## 在业务代码中判断错误

```go
result, err := cli.Call(ctx, "UserService", "GetUser", req)
if err != nil {
    // 方式一：errors.Is 检查标准 Go error
    if errors.Is(err, context.DeadlineExceeded) {
        // 超时
    }

    // 方式二：检查 sentinel error
    if errors.Is(err, client.ErrServiceUnavailable) {
        // 熔断或服务不可用
    }

    // 方式三：获取错误码（通过类型断言）
    var protoErr *protocol.Error
    if errors.As(err, &protoErr) {
        switch protoErr.Code {
        case protocol.ResourceExhausted:
            time.Sleep(1 * time.Second) // 限流退避
        case protocol.Internal:
            log.Error("server internal error", "msg", protoErr.Message)
        }
    }
}
```

## 相关文档

- [消息格式](message-format.md) — Response.Error 字段
- [熔断器](../reliability/circuit-breaker.md) — Unavailable 错误来源
- [限流器](../reliability/rate-limiter.md) — ResourceExhausted 错误来源
- [重试机制](../reliability/retry.md) — 哪些错误码可安全重试
