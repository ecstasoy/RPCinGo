# JSON Codec

## 概述

JSON Codec 使用 Go 标准库 `encoding/json` 实现，是最通用的序列化格式。实现比较复杂（314 行），核心挑战在于正确处理 `Args`/`Data` 字段的嵌套编码。

**源码位置**：`pkg/codec/json.go`

## 核心逻辑：Payload 自适应处理

JSON Codec 需要处理一个棘手问题：`Request.Args` 和 `Response.Data` 的类型是 `interface{}`，其内容可能是：

1. 原始 Go 对象（结构体、map）→ 需要 `json.Marshal`
2. 已经被 Protobuf 序列化的 `[]byte` → 应嵌入为 JSON base64 或直接存储

框架通过 `ArgsCodec`/`DataCodec` 字段标记内容的编码类型，JSON Codec 据此决定如何处理：

```go
// 编码 Request（简化版）
func (c *JSONCodec) EncodeRequest(req *protocol.Request) ([]byte, error) {
    wrapper := &jsonRequest{
        ID:             req.ID,
        Service:        req.Service,
        Method:         req.Method,
        ServiceVersion: req.ServiceVersion,
        Timeout:        req.Timeout,
        IsStream:       req.IsStream,
        Metadata:       req.Metadata,
        CreatedAt:      req.CreatedAt,
        ArgsCodec:      req.ArgsCodec,
    }

    // Args 字段智能处理
    switch v := req.Args.(type) {
    case []byte:
        // 已经是字节数组（如 Protobuf 序列化结果），直接嵌入
        wrapper.Args = json.RawMessage(v)
    case json.RawMessage:
        wrapper.Args = v
    default:
        // 普通 Go 对象，序列化为 JSON
        argsBytes, err := json.Marshal(v)
        if err != nil {
            return nil, err
        }
        wrapper.Args = json.RawMessage(argsBytes)
    }

    return json.Marshal(wrapper)
}
```

## 解码时的类型恢复

解码 Request 时，`Args` 字段会被解码为 `json.RawMessage`，需要调用方进一步解码为具体类型：

```go
// 服务端 handler 接收到的 req.Args 类型
switch req.ArgsCodec {
case pb.PayloadCodec_JSON:
    // Args 是 json.RawMessage，使用 json.Unmarshal
    json.Unmarshal(req.Args.(json.RawMessage), &myRequest)

case pb.PayloadCodec_PROTOBUF:
    // Args 是 []byte，使用 proto.Unmarshal
    proto.Unmarshal(req.Args.([]byte), &myProtoRequest)
}
```

框架的 `ServiceRegistry` 会自动处理这个转换，应用层 handler 直接接收已解码的 Go 对象。

## 基础接口实现

```go
type JSONCodec struct{}

func (c *JSONCodec) Encode(v interface{}) ([]byte, error) {
    return json.Marshal(v)
}

func (c *JSONCodec) Decode(data []byte, v interface{}) error {
    return json.Unmarshal(data, v)
}

func (c *JSONCodec) Name() string {
    return "json"
}
```

## 注册

```go
func init() {
    codec.Register(protocol.CodecTypeJSON, &JSONCodec{})
}
```

## 配置 Server/Client 使用 JSON

```go
// 服务端
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone),
)

// 客户端
cli, _ := client.NewClient("127.0.0.1:8080",
    client.WithCodec(protocol.CodecTypeJSON),
    client.WithCompress(protocol.CompressTypeNone),
)
```

## 与 Gzip 组合

JSON 文本压缩效果好（通常 60–80% 的压缩率），适合与 Gzip 配合使用：

```go
// Gzip 压缩 JSON
srv := server.NewServer(
    server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeGzip),
)
```

框架内部自动创建 `CompressedCodec{inner: JSONCodec, compressor: GzipCompressor}` 处理编解码。

## 性能数据

对于 100 字节的小消息：
- JSON 编码：~100ns
- JSON 解码：~200ns（`json.Unmarshal` 使用反射）

对于 10KB 的消息：
- JSON 编码：~3µs
- JSON 解码：~8µs
- JSON + Gzip：+0.5ms 压缩/0.3ms 解压

## 适用场景总结

| 场景 | 推荐 |
|------|------|
| 开发/调试（人类可读） | ✅ JSON |
| 低 QPS（< 5k） | ✅ JSON |
| 动态类型参数（`interface{}`/`map`) | ✅ JSON |
| 高 QPS（> 50k） | ❌ 用 Protobuf |
| 极小消息体积 | ❌ 用 Protobuf |

## 相关文档

- [Protobuf Codec](protobuf.md)
- [Codec 概述](overview.md) — 注册表与压缩装饰器
- [编解码类型](../protocol/codec-types.md)
