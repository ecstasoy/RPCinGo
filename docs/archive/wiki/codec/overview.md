# Codec 概述

## 职责

Codec 层将 Go 对象序列化为字节流（网络传输）并将字节流反序列化为 Go 对象。支持可选的 Gzip 压缩，通过装饰器模式透明叠加在任意 Codec 之上。

**源码位置**：`pkg/codec/`

## 两套接口

### Codec（内存缓冲，常用）

```go
// pkg/codec/codec.go
type Codec interface {
    Encode(v interface{}) ([]byte, error)
    Decode(data []byte, v interface{}) error
    Name() string
}
```

用于将完整消息编码到 `[]byte`，再一次性写入网络，适合中小消息。

### StreamCodec（流式读写，减少 GC）

```go
type StreamCodec interface {
    EncodeToWriter(w io.Writer, v interface{}) error
    DecodeFromReader(r io.Reader, v interface{}) error
}
```

直接向 `io.Writer`（TCP 连接）写入，避免中间 `[]byte` 分配，对高 QPS 场景减少 GC 压力。

**Protobuf Codec 使用 4 字节长度前缀帧**（StreamCodec 实现）：

```
[len(4B, big-endian)] [protobuf bytes]
```

这与 gRPC Wire Format 类似，便于在流中分隔多个消息。

## 全局注册表

所有 Codec 通过 `CodecType` 键注册到全局单例，使用 `sync.RWMutex` 保护并发访问：

```go
// pkg/codec/codec.go
var defaultRegistry = &codecRegistry{
    codecs: make(map[protocol.CodecType]Codec),
}

func Register(t protocol.CodecType, c Codec) {
    defaultRegistry.mu.Lock()
    defer defaultRegistry.mu.Unlock()
    defaultRegistry.codecs[t] = c
}

func Get(t protocol.CodecType) Codec {
    defaultRegistry.mu.RLock()
    defer defaultRegistry.mu.RUnlock()
    return defaultRegistry.codecs[t]
}
```

各 Codec 实现在 `init()` 函数中自动注册，应用层无需手动调用：

```go
// pkg/codec/json.go
func init() {
    codec.Register(protocol.CodecTypeJSON, &JSONCodec{})
}

// pkg/codec/protobuf.go
func init() {
    codec.Register(protocol.CodecTypeProtobuf, &ProtobufCodec{})
}
```

## 压缩装饰器（CompressedCodec）

`CompressedCodec` 实现 `Codec` 接口，透明地将压缩叠加在任意 Codec 之上（装饰器模式）：

```go
type CompressedCodec struct {
    inner      Codec
    compressor Compressor
}

// 编码：先序列化，再压缩
func (c *CompressedCodec) Encode(v interface{}) ([]byte, error) {
    data, err := c.inner.Encode(v)
    if err != nil {
        return nil, err
    }
    return c.compressor.Compress(data)
}

// 解码：先解压，再反序列化
func (c *CompressedCodec) Decode(data []byte, v interface{}) error {
    decompressed, err := c.compressor.Decompress(data)
    if err != nil {
        return err
    }
    return c.inner.Decode(decompressed, v)
}
```

创建带压缩的 Codec：

```go
gzipJSON := &codec.CompressedCodec{
    Inner:      codec.Get(protocol.CodecTypeJSON),
    Compressor: codec.GetCompressor(protocol.CompressTypeGzip),
}
```

## Compressor 接口与注册表

```go
// pkg/codec/compress.go
type Compressor interface {
    Compress(data []byte) ([]byte, error)
    Decompress(data []byte) ([]byte, error)
    Name() string
}

func GetCompressor(t protocol.CompressType) Compressor {
    return compressors[t] // 全局 map，初始化时注册
}
```

内置实现：
- `NoneCompressor`：直接透传，不做任何处理
- `GzipCompressor`：`compress/gzip`，支持可配置压缩级别

## JSON Codec 对 Payload 的特殊处理

**源码**：`pkg/codec/json.go`（314 行，是最复杂的 Codec）

JSON Codec 在编解码 Request/Response 时会智能处理 `Args`/`Data` 字段：

- **编码 Request 时**：如果 `Args` 已经是 `[]byte`（表示已被 Protobuf 序列化），直接嵌入 JSON；否则调用 `json.Marshal(Args)`
- **解码 Request 时**：根据 `ArgsCodec` 字段决定用 JSON 还是 Protobuf 解码 `Args` 内容
- **编码 Response 时**：对 `Data` 做同样的自适应处理

这使得框架可以混合使用：消息封装用 JSON，内部 payload 用 Protobuf。

## Protobuf Codec 对 proto.Message 的要求

**源码**：`pkg/codec/protobuf.go`（345 行）

Protobuf Codec 在编码时会做类型检查：

```go
func (c *ProtobufCodec) Encode(v interface{}) ([]byte, error) {
    msg, ok := v.(proto.Message)
    if !ok {
        return nil, fmt.Errorf("protobuf codec: %T does not implement proto.Message", v)
    }
    return proto.Marshal(msg)
}
```

因此，使用 `CodecTypeProtobuf` 时，`Request.Args` 和 `Response.Data` 必须是实现了 `proto.Message` 接口的 Protobuf 生成类型。

## 选择指南

```
小消息（< 1KB）且需调试  → JSON + None
大消息（> 1KB）且带宽有限 → JSON + Gzip
生产高 QPS（> 50k）      → Protobuf + None   ← 推荐
超大 Proto 消息           → Protobuf + Gzip
```

## 相关文档

- [JSON Codec](json.md) — 详细实现与 Payload 自适应处理
- [Protobuf Codec](protobuf.md) — proto.Message 要求与 4 字节帧格式
- [编解码类型](../protocol/codec-types.md) — CodecType/CompressType 枚举
- [设计模式](../architecture/design-patterns.md) — 装饰器模式、Registry 模式
