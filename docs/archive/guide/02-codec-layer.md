# Codec 层文档

## 📋 目录

- [概述](#概述)
- [核心组件](#核心组件)
- [使用指南](#使用指南)
- [性能对比](#性能对比)
- [设计原理](#设计原理)
- [最佳实践](#最佳实践)

---

## 概述

### 什么是 Codec 层？

Codec 层是 RPC 框架的**序列化层**，负责将 Go 对象转换为字节流，以及反向转换。

### 职责

```
✅ 对象序列化（Encode）
✅ 字节流反序列化（Decode）
✅ 流式编解码（StreamCodec）
✅ 数据压缩（Compressor）
✅ 编解码器管理（Registry）
```

---

## 核心组件

### 1. Codec 接口

**定义**：`pkg/codec/codec.go`

```go
type Codec interface {
    // 编码：对象 → 字节流
    Encode(v interface{}) ([]byte, error)
    
    // 解码：字节流 → 对象
    Decode(data []byte, v interface{}) error
    
    // 返回编解码器名称
    Name() string
}
```

**支持的实现**：
- ✅ JSONCodec（基于 encoding/json）
- ✅ ProtobufCodec（基于 google.golang.org/protobuf）

---

### 2. StreamCodec 接口

**定义**：`pkg/codec/codec.go`

**作用**：流式编解码，避免内存拷贝

```go
type StreamCodec interface {
    // 编码并写入 Writer
    EncodeToWriter(w io.Writer, v interface{}) error
    
    // 从 Reader 读取并解码
    DecodeFromReader(r io.Reader, v interface{}) error
}
```

**优势**：

```
普通编解码:
  对象 → []byte (内存) → io.Writer
  内存占用: 2x

流式编解码:
  对象 → io.Writer (直接)
  内存占用: 1x
  
性能提升: 减少 50% 内存分配
```

---

### 3. JSONCodec

**定义**：`pkg/codec/json.go`

**特点**：
- ✅ 基于 Go 标准库 `encoding/json`
- ✅ 可读性强（文本格式）
- ✅ 调试方便
- ✅ 跨语言兼容

**使用示例**：

```go
codec := codec.NewJSONCodec()

// 编码
req := protocol.NewRequest("Service", "Method", args)
data, err := codec.Encode(req)
// data = []byte(`{"id":1,"service":"Service",...}`)

// 解码
var decodedReq protocol.Request
err := codec.Decode(data, &decodedReq)

// 流式编码
var buf bytes.Buffer
err := codec.(codec.StreamCodec).EncodeToWriter(&buf, req)
```

**性能**：
```
编码: 175 ns/op,  96 B/op, 2 allocs/op
解码: 680 ns/op, 320 B/op, 9 allocs/op
```

---

### 4. ProtobufCodec

**定义**：`pkg/codec/protobuf.go`

**特点**：
- ✅ 二进制格式（体积小）
- ✅ 性能高（比 JSON 快）
- ✅ 强类型
- ✅ 跨语言

**设计：混合序列化**

```
外层: Protobuf (高效)
内层: JSON (处理 interface{})

Request {
    service: "User"     ← Protobuf
    method: "GetUser"   ← Protobuf
    args: "{\"id\":123}"  ← JSON (因为是 interface{})
}
```

**使用示例**：

```go
codec := codec.NewProtobufCodec()

// 编码
data, err := codec.Encode(req)

// 解码
var resp protocol.Response
err := codec.Decode(data, &resp)
```

**性能**：
```
编码: 427 ns/op, 304 B/op,  6 allocs/op
解码: 645 ns/op, 872 B/op, 14 allocs/op

往返: 1644 ns/op (vs JSON 1888 ns/op)
提升: 约 13%
```

---

### 5. Compressor（压缩器）

**定义**：`pkg/codec/compress.go`

**作用**：压缩和解压数据

#### 接口

```go
type Compressor interface {
    Compress(data []byte) ([]byte, error)
    Decompress(data []byte) ([]byte, error)
    Name() string
}
```

#### 实现

```go
// 不压缩
NoneCompressor

// Gzip 压缩
GzipCompressor (支持 9 个压缩级别)
```

**使用示例**：

```go
// 获取压缩器
compressor := codec.GetCompressor(protocol.CompressTypeGzip)

// 压缩
compressed, err := compressor.Compress(data)

// 解压
decompressed, err := compressor.Decompress(compressed)
```

**压缩效果**：

```
测试数据: 重复文本 (1400 bytes)
压缩后:   58 bytes
压缩率:   95.86%

不同级别:
  NoCompression:      -0.71% (几乎不压缩)
  BestSpeed:          93.29% (快速，压缩率高)
  DefaultCompression: 95.86% (平衡)
  BestCompression:    95.86% (最高，但慢)
```

---

### 6. CompressedCodec（装饰器）

**定义**：`pkg/codec/codec.go`

**作用**：组合 Codec + Compressor

```go
// 创建带压缩的编解码器
jsonCodec := codec.NewJSONCodec()
gzipCompressor := codec.GetCompressor(protocol.CompressTypeGzip)

compressedCodec := codec.NewCompressedCodec(jsonCodec, gzipCompressor)

// 使用（自动压缩）
data, _ := compressedCodec.Encode(req)
// data 是压缩后的
```

**装饰器模式**：
```
CompressedCodec {
    codec.Encode()      → []byte
        ↓
    compressor.Compress() → 压缩后的 []byte
}
```

---

## 性能对比

### JSON vs Protobuf

| 操作 | JSON | Protobuf | 差异 |
|------|------|----------|------|
| **编码** | 175 ns | 427 ns | Protobuf 慢 2.4x |
| **解码** | 680 ns | 645 ns | 基本相同 |
| **往返** | 1888 ns | 1644 ns | Protobuf 快 13% |
| **内存分配** | 2 次 | 6 次 | JSON 更少 |

**结论**：
- JSON：编码快，适合频繁发送
- Protobuf：体积小，适合大数据

### 压缩效果

| 数据类型 | 原始大小 | 压缩后 | 压缩率 |
|---------|---------|--------|--------|
| JSON 文本 | 3698 bytes | 383 bytes | 89.64% |
| 重复文本 | 1400 bytes | 58 bytes | 95.86% |
| 随机数据 | 1000 bytes | 980 bytes | 2% |

**建议**：
- 文本数据：应该压缩
- 二进制数据：视情况而定
- < 1KB 数据：不建议压缩

---

## 设计原理

### 1. 注册表模式

**为什么用注册表？**

```go
// 集中管理
var registry = map[CodecType]Codec{
    CodecTypeJSON:     &JSONCodec{},
    CodecTypeProtobuf: &ProtobufCodec{},
}

// 动态获取
codec := codec.Get(codecType)

// 好处：
- 插件化（易于扩展）
- 运行时选择（灵活）
- 解耦（使用方不依赖具体实现）
```

### 2. 混合序列化策略

**为什么 Protobuf 内部用 JSON？**

```
问题：
  Protobuf 是强类型
  Go 的 interface{} 是任意类型
  如何序列化 interface{}？

解决方案：
  外层：Protobuf（高效的结构）
  内层：JSON（灵活的 interface{}）
  
  pb.Request {
      service: "User"        ← Protobuf
      args: []byte(JSON)     ← JSON 序列化的 interface{}
  }

好处：
  - 兼顾性能和灵活性
  - Protobuf 节省外层空间
  - JSON 处理动态类型
```

---

## 最佳实践

### 1. 选择合适的编解码器

```go
// JSON：调试、开发
codec := codec.Get(protocol.CodecTypeJSON)

// Protobuf：生产、性能
codec := codec.Get(protocol.CodecTypeProtobuf)
```

### 2. 何时使用压缩

```go
// 大数据（> 1KB）
if len(data) > 1024 {
    compressor := codec.GetCompressor(protocol.CompressTypeGzip)
    compressed, _ := compressor.Compress(data)
}

// 小数据（< 1KB）：不压缩
// 原因：压缩开销 > 收益
```

### 3. 流式编解码

```go
// 大对象：使用流式
var buf bytes.Buffer
codec.(codec.StreamCodec).EncodeToWriter(&buf, largeObject)

// 小对象：普通编解码
data, _ := codec.Encode(smallObject)
```

---

## 测试覆盖

```
测试用例: 26 个
覆盖率:   80.0%
测试类型:
  - 单元测试（编解码正确性）
  - 性能测试（Benchmark）
  - 集成测试（Protocol + Codec）
  - 边界测试（nil、空值）
```

---

## 依赖关系

```
Codec 层:
  依赖:
    - Protocol 层（使用 CodecType、CompressType）
    - Go 标准库（encoding/json）
    - google.golang.org/protobuf
  
  被依赖:
    - Transport 层
```

---

**文档版本**: v1.0  
**最后更新**: 2026-01-02  
**作者**: Kunhua Huang





