# 消息格式

## 概述

每个 RPC 调用由一个 **Request**（客户端 → 服务端）和一个 **Response**（服务端 → 客户端）组成。两者均以 [协议头（20 字节）](header.md) + 序列化 Body 的格式在 TCP 连接上传输。

**源码位置**：`pkg/protocol/request.go`、`pkg/protocol/response.go`

## Request 结构

```go
// pkg/protocol/request.go
type Request struct {
    ID             uint64       // 请求唯一 ID，由全局原子计数器生成
    Service        string       // 目标服务名，如 "UserService"
    Method         string       // 目标方法名，如 "GetUser"
    ServiceVersion string       // 服务版本（可选），用于多版本路由
    Args           interface{}  // 请求参数，序列化前的 Go 对象
    Timeout        int64        // 超时时间（毫秒），0 = 不限
    IsStream       bool         // 是否流式（当前预留，默认 false）
    Metadata       Metadata     // 键值对元数据，随请求透传
    CreatedAt      int64        // 请求创建时间（Unix 毫秒）
    ArgsCodec      PayloadCodec // Args 字段的编码类型（JSON/Protobuf）
}
```

### 请求 ID 生成机制

```go
var globalID uint64

// pkg/protocol/request.go
func NewRequest(service, method string, args interface{}) *Request {
    return &Request{
        ID:        atomic.AddUint64(&globalID, 1), // 原子自增，进程内唯一
        Service:   service,
        Method:    method,
        Args:      args,
        CreatedAt: time.Now().UnixMilli(),
    }
}
```

原子自增从 1 开始，在单进程内永不重复。与 TCP 连接的有序性结合，可正确匹配请求与响应。

### Timeout 语义

`Timeout` 字段由客户端填写，表示从请求创建（`CreatedAt`）到期望服务端完成的毫秒数。服务端可用此值设置 handler 的 `context.WithDeadline`：

```go
deadline := time.UnixMilli(req.CreatedAt + req.Timeout)
ctx, cancel := context.WithDeadline(ctx, deadline)
defer cancel()
```

如果 `Timeout == 0`，服务端使用自身配置的 `ReadTimeout` 作为截止时间。

## Response 结构

```go
// pkg/protocol/response.go
type Response struct {
    ID         uint64       // 与对应 Request.ID 相同，用于匹配
    Data       interface{}  // 响应数据，Codec 解码后的 Go 对象
    Error      *Error       // 非 nil 表示调用失败
    Metadata   Metadata     // 服务端返回的元数据（如 Trace-ID、服务版本）
    ServerTime int64        // 服务端处理完成时间（Unix 毫秒）
    DataCodec  PayloadCodec // Data 字段的编码类型
}
```

### 成功/失败判断

```go
func (r *Response) IsSuccess() bool {
    return r.Error == nil || r.Error.Code == OK
}
```

客户端收到 Response 后，先检查 `Error` 字段，再使用 `Data` 字段。

## Payload Codec 区分

协议头中的 `Codec` 和消息结构体中的 `ArgsCodec`/`DataCodec` 含义不同：

| 字段 | 描述 |
|------|------|
| `Header.Codec` | 整个消息体（Request 或 Response 结构）的序列化格式 |
| `Request.ArgsCodec` | 仅 `Args` 字段内容的序列化格式 |
| `Response.DataCodec` | 仅 `Data` 字段内容的序列化格式 |

`PayloadCodec` 枚举定义在 `pkg/protocol/pb/` 的 Protobuf 文件中：

```protobuf
enum PayloadCodec {
    JSON     = 0;
    PROTOBUF = 1;
}
```

这种双层编码允许以下场景：整个消息用 JSON 传输（兼容性好），但 Args/Data 内容用 Protobuf 编码（体积小）。

## 网络上的完整消息

```
TCP 字节流中的单个消息（以 Request 为例）：

┌──────────────────────────────────────────────────────┐
│  Header（20 字节固定）                                │
│  0xCAFE │ ver=1 │ type=1 │ codec=1 │ compress=0 │... │
│  RequestID=42 │ BodyLength=156                       │
├──────────────────────────────────────────────────────┤
│  Body（156 字节，JSON 编码的 Request 结构体）          │
│  {"id":42,"service":"UserService","method":"GetUser" │
│   ,"args":{"user_id":1001},"timeout":5000,...}       │
└──────────────────────────────────────────────────────┘
```

## JSON 序列化示例

以 JSON Codec 为例，Body 内容（`encoding/json.Marshal(request)`）：

```json
{
  "id": 42,
  "service": "UserService",
  "method": "GetUser",
  "service_version": "",
  "args": {"user_id": 1001},
  "timeout": 5000,
  "is_stream": false,
  "metadata": {
    "trace-id": "abc-123",
    "x-user-id": "999"
  },
  "created_at": 1711785600000,
  "args_codec": 0
}
```

对应的 Response：

```json
{
  "id": 42,
  "data": {"user_id": 1001, "name": "Alice", "email": "alice@example.com"},
  "error": null,
  "metadata": {"server-version": "1.0.0"},
  "server_time": 1711785600012,
  "data_codec": 0
}
```

## 相关文档

- [协议头](header.md) — 20 字节头部格式
- [编解码类型](codec-types.md) — Codec/Compress 枚举
- [Metadata](metadata.md) — 标准元数据键
- [错误码](error-codes.md) — Error 结构详解
