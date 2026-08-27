# 协议头

## 概述

RPCinGo 使用 **20 字节固定长度**的协议头，所有多字节字段采用**大端字节序**（网络字节序）。固定长度使服务端可以用一次 `io.ReadFull(conn, buf[:20])` 精确读取，无需任何分隔符或额外的帧界定逻辑。

**源码位置**：`pkg/protocol/header.go`

## 字节布局

```
字节偏移:  0    1    2    3    4    5    6    7
         ┌────┬────┬────┬────┬────┬────┬────┬────┐
         │  Magic(2B) │Ver │Type│Cdc │Cmp │  Reserved(2B)  │
         └────┴────┴────┴────┴────┴────┴────┴────┘

字节偏移:  8    9   10   11   12   13   14   15
         ┌────┬────┬────┬────┬────┬────┬────┬────┐
         │              RequestID (8B, uint64)    │
         └────┴────┴────┴────┴────┴────┴────┴────┘

字节偏移: 16   17   18   19
         ┌────┬────┬────┬────┐
         │   BodyLength (4B) │
         └────┴────┴────┴────┘
```

## 字段说明

| 字段 | 偏移 | 大小 | Go 类型 | 说明 |
|------|------|------|---------|------|
| `Magic` | 0 | 2 字节 | `uint16` | 魔数 `0xCAFE`，协议识别标志 |
| `Version` | 2 | 1 字节 | `uint8` | 协议版本，当前为 `1` |
| `MsgType` | 3 | 1 字节 | `MessageType` | 消息类型：Request=`1` / Response=`2` |
| `Codec` | 4 | 1 字节 | `CodecType` | 序列化格式（见下方枚举） |
| `Compress` | 5 | 1 字节 | `CompressType` | 压缩算法（见下方枚举） |
| `Reserved` | 6–7 | 2 字节 | `[2]byte` | 保留，当前全零，供未来版本扩展 |
| `RequestID` | 8–15 | 8 字节 | `uint64` | 请求唯一标识，客户端原子自增 |
| `BodyLength` | 16–19 | 4 字节 | `uint32` | Body 字节数，服务端按此长度读取 |

**总大小：20 字节（`HeaderSize` 常量）**

## 枚举值

### MessageType

```go
const (
    MessageTypeRequest  MessageType = 1
    MessageTypeResponse MessageType = 2
)
```

### CodecType

```go
const (
    CodecTypeJSON     CodecType = 1
    CodecTypeProtobuf CodecType = 2
    CodecTypeMsgpack  CodecType = 3  // 预留，未完整实现
)
```

### CompressType

```go
const (
    CompressTypeNone   CompressType = 0
    CompressTypeGzip   CompressType = 1
    CompressTypeSnappy CompressType = 2  // 预留
)
```

> 详细的 Codec 和 Compress 类型说明见 [编解码类型](codec-types.md)。

## 读写实现

### 写入（ProtocolCodec.encodeHeader）

```go
// pkg/transport/tcp/codec.go
func encodeHeader(h *protocol.Header) []byte {
    buf := make([]byte, protocol.HeaderSize) // 20 字节
    binary.BigEndian.PutUint16(buf[0:2], h.Magic)
    buf[2] = h.Version
    buf[3] = byte(h.MsgType)
    buf[4] = byte(h.Codec)
    buf[5] = byte(h.Compress)
    // buf[6:8] 保留，已初始化为零
    binary.BigEndian.PutUint64(buf[8:16], h.RequestID)
    binary.BigEndian.PutUint32(buf[16:20], h.BodyLength)
    return buf
}
```

### 读取（ProtocolCodec.decodeHeader）

```go
func decodeHeader(buf []byte) (*protocol.Header, error) {
    magic := binary.BigEndian.Uint16(buf[0:2])
    if magic != protocol.Magic { // 0xCAFE
        return nil, fmt.Errorf("invalid magic number: 0x%X", magic)
    }
    return &protocol.Header{
        Magic:      magic,
        Version:    buf[2],
        MsgType:    protocol.MessageType(buf[3]),
        Codec:      protocol.CodecType(buf[4]),
        Compress:   protocol.CompressType(buf[5]),
        RequestID:  binary.BigEndian.Uint64(buf[8:16]),
        BodyLength: binary.BigEndian.Uint32(buf[16:20]),
    }, nil
}
```

### 服务端读取两阶段流程

```go
// 阶段 1：精确读取固定 20 字节头
headerBuf := make([]byte, protocol.HeaderSize)
if _, err := io.ReadFull(conn, headerBuf); err != nil {
    return // 连接关闭或异常
}
header, err := decodeHeader(headerBuf)

// 阶段 2：按头中的 BodyLength 读取变长 Body
bodyBuf := make([]byte, header.BodyLength)
if _, err := io.ReadFull(conn, bodyBuf); err != nil {
    return
}
// 此后交给 Codec 解码 bodyBuf
```

## 魔数校验

每个新连接的第一个读操作就会验证魔数。魔数不匹配时立即关闭连接，常见原因：

- HTTP 客户端误连到 RPC 端口
- 数据损坏或字节序错误
- 版本不兼容（未来版本可通过 Version 字段区分）

## 设计决策

**为什么是固定 20 字节而非可变长头？**
固定长度头可以用单次 `io.ReadFull` 读取，无需解析分隔符，避免缓冲区复杂性，也不需要像 HTTP/2 那样的帧状态机。

**为什么 RequestID 是 8 字节（uint64）？**
保证即使在极高 QPS（1M req/s）下，溢出需要 584,000 年，实际上永不碰撞。同一 TCP 连接上的响应通过 RequestID 与请求匹配。

**为什么保留 2 字节？**
为将来扩展预留（例如 Priority 字段、流控标志），不破坏当前协议的解析代码。

## 相关文档

- [编解码类型](codec-types.md) — Codec 和 Compress 枚举完整说明
- [消息格式](message-format.md) — Request / Response Body 结构
- [TCP 传输](../transport/tcp.md) — 两阶段读取的完整实现
- [数据流](../architecture/data-flow.md) — 端到端编解码流程
