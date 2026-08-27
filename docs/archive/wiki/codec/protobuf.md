# Protobuf Codec

## 概述

Protobuf Codec 使用 `google.golang.org/protobuf`（v2 API）实现，是生产环境推荐的序列化格式。结合 `CallTyped()` 接口可实现完整的编译期类型安全。

**源码位置**：`pkg/codec/protobuf.go`（345 行）

## 两种使用模式

### 模式一：普通 Codec 接口（`[]byte` 缓冲）

```go
func (c *ProtobufCodec) Encode(v interface{}) ([]byte, error) {
    msg, ok := v.(proto.Message)
    if !ok {
        return nil, fmt.Errorf("protobuf codec: %T does not implement proto.Message", v)
    }
    return proto.Marshal(msg)
}

func (c *ProtobufCodec) Decode(data []byte, v interface{}) error {
    msg, ok := v.(proto.Message)
    if !ok {
        return fmt.Errorf("protobuf codec: %T does not implement proto.Message", v)
    }
    return proto.Unmarshal(data, msg)
}
```

### 模式二：StreamCodec 接口（4 字节长度前缀帧）

```go
// 写入格式：[len(4B, big-endian)] [protobuf bytes]
func (c *ProtobufCodec) EncodeToWriter(w io.Writer, v interface{}) error {
    msg := v.(proto.Message)
    data, err := proto.Marshal(msg)
    if err != nil {
        return err
    }
    // 写入 4 字节长度头
    lenBuf := make([]byte, 4)
    binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
    if _, err := w.Write(lenBuf); err != nil {
        return err
    }
    _, err = w.Write(data)
    return err
}

func (c *ProtobufCodec) DecodeFromReader(r io.Reader, v interface{}) error {
    msg := v.(proto.Message)
    // 读取 4 字节长度
    lenBuf := make([]byte, 4)
    if _, err := io.ReadFull(r, lenBuf); err != nil {
        return err
    }
    length := binary.BigEndian.Uint32(lenBuf)
    // 读取消息体
    data := make([]byte, length)
    if _, err := io.ReadFull(r, data); err != nil {
        return err
    }
    return proto.Unmarshal(data, msg)
}
```

StreamCodec 的 4 字节帧格式与 gRPC Wire Format 类似，便于在 TCP 流中分隔多个 Protobuf 消息。

## `proto.Message` 要求

使用 Protobuf Codec 时，`Request.Args` 和 `Response.Data` 必须实现 `proto.Message` 接口，即必须是由 `protoc-gen-go` 生成的类型：

```go
// ✅ 正确：Protobuf 生成类型
req := &calculator.AddRequest{A: 10, B: 20}
cli.CallTyped(ctx, "Calculator", "Add", req, resp)

// ❌ 错误：普通 Go struct
req := &AddRequest{A: 10, B: 20}  // 不实现 proto.Message
cli.CallTyped(ctx, "Calculator", "Add", req, resp) // 编译通过但运行时 panic
```

## `CallTyped()` 的内部流程

**源码**：`pkg/client/client.go`

```go
func (c *Client) CallTyped(ctx context.Context,
    service, method string,
    req proto.Message, resp proto.Message) error {

    // 1. 序列化 req
    argsBytes, err := proto.Marshal(req)
    if err != nil {
        return err
    }

    // 2. 构建 Request，Args 存储已序列化的 []byte
    rpcReq := &protocol.Request{
        ID:        atomic.AddUint64(&globalID, 1),
        Service:   service,
        Method:    method,
        Args:      argsBytes,
        ArgsCodec: pb.PayloadCodec_PROTOBUF,
    }

    // 3. 发送请求，接收响应
    result, err := c.call(ctx, rpcReq)
    if err != nil {
        return err
    }

    // 4. 将 result 解码到 resp
    if dataBytes, ok := result.([]byte); ok {
        return proto.Unmarshal(dataBytes, resp)
    }
    return fmt.Errorf("unexpected result type: %T", result)
}
```

## 注册

```go
func init() {
    codec.Register(protocol.CodecTypeProtobuf, &ProtobufCodec{})
}
```

## 生成 Protobuf 代码

项目提供了生成脚本：

```bash
# 查看脚本内容
cat scripts/gen-example-proto.sh

# 生成 examples/calculator 的 proto
bash scripts/gen-example-proto.sh
```

示例 proto 文件位于 `examples/calculator/` 和 `pkg/protocol/pb/`。

## 服务端处理 Protobuf 请求

框架的 `ServiceRegistry` 会自动检测方法参数类型是否为 `proto.Message`，并使用 `proto.Unmarshal` 反序列化 `Args`：

```go
// 服务端方法签名（框架自动处理反序列化）
func (s *CalculatorService) Add(ctx context.Context,
    req *calculator.AddRequest) (*calculator.AddResponse, error) {
    // req 已经是完全解码的 Protobuf 对象，直接使用
    return &calculator.AddResponse{Result: req.A + req.B}, nil
}
```

详见 [服务注册](../server/service-registration.md) 中对 Typed 方法的说明。

## 性能对比

| 操作 | JSON | Protobuf |
|------|------|----------|
| 编码 1KB 消息 | ~3µs | ~0.5µs（6x 快）|
| 解码 1KB 消息 | ~8µs | ~1µs（8x 快）|
| 消息体积 | 1KB | ~250B（4x 小）|
| 压缩后体积 | ~300B | ~200B |

数据来源：框架 benchmark 测试（`pkg/codec/protobuf_test.go`）。

## 相关文档

- [JSON Codec](json.md)
- [Codec 概述](overview.md)
- [Calculator 示例](../getting-started/calculator-example.md) — 端到端 Protobuf 示例
- [服务注册](../server/service-registration.md) — Typed 方法签名
