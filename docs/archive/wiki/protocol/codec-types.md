# 编解码类型

## 序列化格式（CodecType）

**源码位置**：`pkg/protocol/header.go`

```go
type CodecType uint8

const (
    CodecTypeJSON     CodecType = 1  // encoding/json
    CodecTypeProtobuf CodecType = 2  // google.golang.org/protobuf
    CodecTypeMsgpack  CodecType = 3  // 预留，当前未完整实现
)
```

### JSON（CodecType = 1）

- 使用 Go 标准库 `encoding/json`
- 无 schema 依赖，调试友好，可用 `curl` 等工具测试
- 支持 `interface{}` 类型参数，无需预定义结构体
- **性能**：编码约 300ns，解码约 500ns（取决于消息大小）
- **适用**：开发调试、内部服务、需要人类可读性

### Protobuf（CodecType = 2）

- 使用 `google.golang.org/protobuf`（v2 API）
- 需要预编译 `.proto` 文件生成 Go 代码
- 体积通常比 JSON 小 2–5 倍
- **性能**：比 JSON 快 3–10 倍（编解码均更快）
- **适用**：生产环境、高 QPS 场景、强类型约束

### MsgPack（CodecType = 3）

预留字段，当前 Codec 注册表中尚未注册 MsgPack 实现。如需使用，可自行实现 `Codec` 接口并调用 `codec.Register(CodecTypeMsgpack, myMsgpackCodec)` 注册。

## 压缩算法（CompressType）

**源码位置**：`pkg/protocol/header.go`，`pkg/codec/compress.go`

```go
type CompressType uint8

const (
    CompressTypeNone   CompressType = 0  // 不压缩
    CompressTypeGzip   CompressType = 1  // compress/gzip
    CompressTypeSnappy CompressType = 2  // 预留
)
```

### 无压缩（CompressType = 0）

- `NoneCompressor`：直接返回原始数据，无任何处理
- 适用于小消息（< 1KB）或延迟敏感场景
- CPU 开销：零

### Gzip（CompressType = 1）

- `GzipCompressor`：使用 `compress/gzip`
- 源码：`pkg/codec/compress.go`

```go
type GzipCompressor struct {
    Level int // gzip.BestSpeed(1) ~ gzip.BestCompression(9), 默认 gzip.DefaultCompression(-1)
}

func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
    var buf bytes.Buffer
    w, _ := gzip.NewWriterLevel(&buf, c.Level)
    w.Write(data)
    w.Close()
    return buf.Bytes(), nil
}

func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
    r, _ := gzip.NewReader(bytes.NewReader(data))
    defer r.Close()
    return io.ReadAll(r)
}
```

**适用场景**：
- 大消息（> 1KB），尤其是 JSON 文本（压缩率通常 60–80%）
- 网络带宽受限的环境
- 消息体包含重复字段（如 ListUsers 返回大量相似 JSON）

**性能权衡**：Gzip 压缩约增加 0.1–1ms CPU 时间，节省带宽，适合网络 IO 比 CPU 更贵的场景。

### Snappy（CompressType = 2）

预留字段，当前未实现。Snappy 比 Gzip 快 5–10 倍，压缩率略低，适合延迟敏感但仍需压缩的场景。

## 组合矩阵

| Codec | Compress | 适用场景 |
|-------|----------|---------|
| JSON | None | 开发调试、小消息 |
| JSON | Gzip | 大文本消息、带宽受限 |
| Protobuf | None | 生产高 QPS **推荐** |
| Protobuf | Gzip | 超大 Protobuf 消息 |

## 在协议头中的位置

这两个字段各占协议头 1 字节：

```
Header[4] = Codec type  (CodecType, 1 byte)
Header[5] = Compress type (CompressType, 1 byte)
```

服务端收到请求后，先从 Header 中读取这两个值，再用对应的 Codec + Decompressor 解码 Body。服务端响应时，使用服务端自身配置的 Codec/Compress（不必与请求保持一致，但通常相同）。

## Compressor 注册表

与 Codec 类似，Compressor 也有全局注册表：

```go
// pkg/codec/compress.go
var compressors = map[protocol.CompressType]Compressor{
    protocol.CompressTypeNone: &NoneCompressor{},
    protocol.CompressTypeGzip: &GzipCompressor{Level: gzip.DefaultCompression},
}

func GetCompressor(t protocol.CompressType) Compressor {
    return compressors[t]
}
```

## 相关文档

- [协议头](header.md) — Codec/Compress 在头部的位置
- [Codec 概述](../codec/overview.md) — 编解码注册机制
- [JSON Codec](../codec/json.md)
- [Protobuf Codec](../codec/protobuf.md)
