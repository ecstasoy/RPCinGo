# Metadata（请求元数据）

## 概述

`Metadata` 是随 Request/Response 透明传递的字符串键值对，用于传递与业务逻辑无关但对基础设施（链路追踪、认证、灰度路由）重要的上下文信息。

**源码位置**：`pkg/protocol/metadata.go`

## 类型定义

```go
type Metadata map[string]string
```

## 方法

```go
// 获取值（键不存在时返回空字符串）
func (m Metadata) Get(key string) string

// 设置值
func (m Metadata) Set(key, value string)

// 深拷贝，防止调用方修改影响原始数据
func (m Metadata) Clone() Metadata

// 合并另一个 Metadata（other 的值覆盖 m 中同名键）
func (m Metadata) Merge(other Metadata) Metadata
```

## 标准键（预定义常量）

框架在 `pkg/protocol/metadata.go` 中定义了以下标准键，使用这些常量而非裸字符串可避免拼写错误：

```go
const (
    MetadataKeyTraceID = "trace-id"     // 分布式链路追踪 ID
    MetadataKeySpanID  = "span-id"      // 当前 Span ID
    MetadataKeyToken   = "x-token"      // 认证 Token（Bearer 或 API Key）
    MetadataKeyUserID  = "x-user-id"    // 调用方用户 ID
    MetadataKeyRegion  = "x-region"     // 区域（如 "us-east-1"）
    MetadataKeyZone    = "x-zone"       // 可用区（如 "az-a"）
    MetadataKeyDebug   = "x-debug"      // 调试模式（"true"/"false"）
)
```

## 使用示例

### 客户端填写 Metadata

```go
req := protocol.NewRequest("UserService", "GetUser", args)
req.Metadata = protocol.Metadata{
    protocol.MetadataKeyTraceID: "550e8400-e29b-41d4-a716-446655440000",
    protocol.MetadataKeyToken:   "Bearer eyJhbGciOiJIUzI1NiJ9...",
    protocol.MetadataKeyUserID:  "1001",
    protocol.MetadataKeyRegion:  "cn-north-1",
}
```

### 服务端读取 Metadata

```go
func (s *UserService) GetUser(ctx context.Context,
    req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {

    // 通常通过 context 传递，由拦截器注入
    rpcReq := ctx.Value("rpc-request").(*protocol.Request)

    traceID := rpcReq.Metadata.Get(protocol.MetadataKeyTraceID)
    token   := rpcReq.Metadata.Get(protocol.MetadataKeyToken)
    debug   := rpcReq.Metadata.Get(protocol.MetadataKeyDebug) == "true"

    // ...
}
```

### 服务端在 Response 中返回 Metadata

```go
resp := &protocol.Response{
    ID:   req.ID,
    Data: result,
    Metadata: protocol.Metadata{
        "server-version": "1.2.3",
        "server-region":  "cn-north-1",
        protocol.MetadataKeyTraceID: traceID, // 回传同一 Trace ID
    },
}
```

## 在拦截器中使用 Metadata

Logging 拦截器自动从 Metadata 中提取 Trace ID，关联日志：

```go
func NewLoggingInterceptor(logger Logger) Interceptor {
    return func(ctx context.Context, req *protocol.Request, next Invoker) (interface{}, error) {
        traceID := req.Metadata.Get(protocol.MetadataKeyTraceID)
        start := time.Now()

        result, err := next(ctx, req)

        logger.Infof("[%s] %s.%s took %v err=%v",
            traceID, req.Service, req.Method, time.Since(start), err)
        return result, err
    }
}
```

## 与 HTTP Header 的类比

Metadata 类似 HTTP 请求头，传递协议级别的上下文信息：

| HTTP Header | RPCinGo Metadata Key |
|-------------|---------------------|
| `X-Trace-ID` | `MetadataKeyTraceID` |
| `Authorization` | `MetadataKeyToken` |
| `X-User-ID` | `MetadataKeyUserID` |
| `X-Region` | `MetadataKeyRegion` |

## 注意事项

- Metadata 随每个请求序列化传输，避免放入大量数据（建议每个 key-value pair 不超过 1KB）
- `Clone()` 在需要修改 Metadata 副本时使用，避免竞态条件
- 框架本身不对 Metadata 内容做验证，业务逻辑需自行校验 Token 等安全字段

## 相关文档

- [消息格式](message-format.md) — Metadata 在 Request/Response 中的位置
- [拦截器链](../server/interceptors.md) — 日志拦截器提取 Trace ID
