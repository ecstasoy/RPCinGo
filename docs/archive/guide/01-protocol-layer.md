# Protocol 层文档

## 📋 目录

- [概述](#概述)
- [核心组件](#核心组件)
- [使用指南](#使用指南)
- [设计原理](#设计原理)
- [API 参考](#api-参考)
- [最佳实践](#最佳实践)

---

## 概述

### 什么是 Protocol 层？

Protocol 层是 RPC 框架的**消息定义层**，定义了客户端和服务端通信的数据结构。

### 职责

```
✅ 定义消息格式（Request/Response）
✅ 定义协议头（Header）
✅ 定义错误结构（Error）
✅ 定义元数据（Metadata）
✅ 提供消息操作方法
```

### 位置

```
应用层
   ↓
RPC 层
   ↓
Protocol 层  ← 【这一层】定义消息格式
   ↓
Codec 层（序列化）
   ↓
Transport 层（网络传输）
```

---

## 核心组件

### 1. Header（协议头）

**定义**：`pkg/protocol/header.go`

**作用**：固定长度的协议头，用于识别和解析消息

#### 结构

```go
type Header struct {
    Magic      uint16       // 魔数 0xCAFE（识别协议）
    Version    byte         // 协议版本
    MsgType    MessageType  // 消息类型（请求/响应）
    Codec      CodecType    // 编解码类型
    Compress   CompressType // 压缩类型
    Reserved   [2]byte      // 预留字段
    RequestID  uint64       // 请求 ID
    BodyLength uint32       // 消息体长度
}
```

#### 字节布局（20 字节）

```
 0  1  2  3  4  5  6  7  8  9  10 11 12 13 14 15 16 17 18 19
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|Magic |Ver|Typ|Cod|Cmp|Reserv |    Request ID     |BodyLen |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
```

#### 使用示例

```go
// 创建 Header
header := protocol.NewHeader(
    protocol.MsgTypeRequest,
    protocol.CodecTypeJSON,
    123,    // Request ID
    1024,   // Body Length
)

// 编码为字节流
data := header.Encode()  // []byte (20 bytes)

// 从字节流解码
header := &protocol.Header{}
err := header.Decode(data)
```

---

### 2. Request（请求消息）

**定义**：`pkg/protocol/request.go`

**作用**：RPC 请求消息，包含调用信息和参数

#### 结构

```go
type Request struct {
    ID             uint64      // 请求唯一标识
    Service        string      // 服务名
    Method         string      // 方法名
    ServiceVersion string      // 服务版本
    Args           interface{} // 方法参数
    Timeout        int64       // 超时时间（毫秒）
    IsStream       bool        // 是否流式调用
    Metadata       Metadata    // 元数据
    CreatedAt      int64       // 创建时间戳
}
```

#### 使用示例

```go
// 创建请求
req := protocol.NewRequest("UserService", "GetUser", map[string]interface{}{
    "id": 123,
})

// 设置超时
req.SetTimeout(5 * time.Second)

// 设置元数据
req.SetMetadata(protocol.MetaKeyTraceID, "trace-123")

// 获取超时
timeout := req.GetTimeout()  // time.Duration
```

---

### 3. Response（响应消息）

**定义**：`pkg/protocol/response.go`

**作用**：RPC 响应消息，包含返回值或错误

#### 结构

```go
type Response struct {
    ID         uint64      // 对应的请求 ID
    Data       interface{} // 返回数据
    Error      *Error      // 错误信息
    Metadata   Metadata    // 元数据
    ServerTime int64       // 服务端处理时间（纳秒）
}
```

#### 使用示例

```go
// 成功响应
resp := protocol.NewSuccessResponse(123, map[string]interface{}{
    "name": "Alice",
    "age":  25,
})

// 错误响应
err := protocol.NewError(protocol.ErrorCodeNotFound, "user not found")
resp := protocol.NewErrorResponse(123, err)

// 检查响应
if resp.IsSuccess() {
    // 处理成功
    data := resp.Data
} else {
    // 处理错误
    errCode := resp.Error.Code
    errMsg := resp.Error.Message
}
```

---

### 4. Error（错误）

**定义**：`pkg/protocol/error.go`

**作用**：结构化的错误信息

#### 结构

```go
type Error struct {
    Code    int32  // 错误码
    Message string // 错误消息
    Details string // 错误详情
}
```

#### 预定义错误码

```go
const (
    ErrorCodeOK                = 0    // 成功
    ErrorCodeCanceled          = 1    // 取消
    ErrorCodeUnknown           = 2    // 未知错误
    ErrorCodeInvalidArgument   = 3    // 无效参数
    ErrorCodeDeadlineExceeded  = 4    // 超时
    ErrorCodeNotFound          = 5    // 未找到
    ErrorCodeInternal          = 13   // 内部错误
    ErrorCodeUnavailable       = 14   // 服务不可用
)
```

---

### 5. Metadata（元数据）

**定义**：`pkg/protocol/metadata.go`

**作用**：传递附加信息（链路追踪、认证等）

#### 类型

```go
type Metadata map[string]string
```

#### 预定义 Key

```go
const (
    MetaKeyTraceID = "trace-id"      // 链路追踪 ID
    MetaKeySpanID  = "span-id"       // Span ID
    MetaKeyToken   = "auth-token"    // 认证令牌
    MetaKeyUserID  = "user-id"       // 用户 ID
    MetaKeyRegion  = "region"        // 地域
    MetaKeyZone    = "zone"          // 可用区
    MetaKeyDebug   = "debug"         // 调试标志
)
```

#### 使用示例

```go
// 创建元数据
meta := protocol.NewMetadata()

// 设置值
meta.Set(protocol.MetaKeyTraceID, "trace-123")
meta.Set(protocol.MetaKeyUserID, "user-456")

// 获取值
traceID, ok := meta.Get(protocol.MetaKeyTraceID)

// 克隆（避免并发修改）
clone := meta.Clone()

// 合并
meta.Merge(otherMeta)

// 转换为 map
m := meta.ToMap()  // map[string]string
```

---

## 使用指南

### 完整的消息生命周期

```go
// 1. 客户端创建请求
req := protocol.NewRequest("UserService", "GetUser", map[string]interface{}{
    "id": 123,
})
req.SetTimeout(5 * time.Second)
req.SetMetadata(protocol.MetaKeyTraceID, "trace-abc")

// 2. 序列化（Codec 层处理）
// 3. 添加 Header（Transport 层处理）
// 4. 网络传输

// 5. 服务端接收并解析
// 6. 处理请求
// 7. 创建响应
resp := protocol.NewSuccessResponse(req.ID, map[string]interface{}{
    "name": "Alice",
})

// 8. 返回给客户端
// 9. 客户端接收响应
if resp.IsSuccess() {
    data := resp.Data.(map[string]interface{})
    name := data["name"].(string)
}
```

---

## 设计原理

### 1. 定长 Header 设计

**为什么用定长？**

```
定长 Header (20 bytes):
✅ 解析简单（直接读取 20 字节）
✅ 性能高（无需查找分隔符）
✅ 避免粘包（知道确切边界）

变长 Header:
❌ 需要寻找结束标志
❌ 解析复杂
❌ 性能较低
```

### 2. 魔数（Magic Number）

**为什么需要魔数？**

```
作用：
1. 协议识别（是否是 RPC 协议）
2. 数据校验（防止错误数据）
3. 版本控制（不同魔数表示不同版本）

0xCAFE 的选择：
- 易识别（CAFE = 咖啡 ☕）
- 不容易与随机数据冲突
- 符合 16 进制惯例
```

### 3. 请求 ID 设计

**为什么需要 ID？**

```
场景：异步 RPC 调用

客户端:
  发送 Request{ID=1, Service="A"}
  发送 Request{ID=2, Service="B"}
  
服务端:
  先处理完 B（快）
  后处理完 A（慢）
  
返回:
  Response{ID=2, ...}  ← 先到
  Response{ID=1, ...}  ← 后到

客户端通过 ID 匹配：
  ID=2 → 是 Service B 的响应
  ID=1 → 是 Service A 的响应
```

**ID 生成策略**：
```go
// 全局计数器 + atomic 操作
var requestIDCounter uint64

func nextRequestID() uint64 {
    return atomic.AddUint64(&requestIDCounter, 1)
}

// 特点：
- 全局唯一
- 并发安全
- 单调递增
```

---

## API 参考

### Header API

```go
// 创建
func NewHeader(msgType MessageType, codec CodecType, 
    requestID uint64, bodyLen uint32) *Header

// 编码
func (h *Header) Encode() []byte

// 解码
func (h *Header) Decode(buf []byte) error

// 格式化
func (h *Header) String() string
```

### Request API

```go
// 创建
func NewRequest(service, method string, args interface{}) *Request
func NewRequestWithVersion(service, method, version string, args interface{}) *Request

// 超时
func (r *Request) SetTimeout(timeout time.Duration)
func (r *Request) GetTimeout() time.Duration

// 元数据
func (r *Request) SetMetadata(key, value string)
func (r *Request) GetMetadata(key string) (string, bool)
```

### Response API

```go
// 创建
func NewResponse(requestID uint64) *Response
func NewSuccessResponse(requestID uint64, data interface{}) *Response
func NewErrorResponse(requestID uint64, err *Error) *Response

// 状态检查
func (r *Response) IsSuccess() bool
func (r *Response) IsError() bool
func (r *Response) GetError() error
```

### Metadata API

```go
// 创建
func NewMetadata() Metadata

// CRUD
func (m Metadata) Set(key, value string)
func (m Metadata) Get(key string) (string, bool)
func (m Metadata) Delete(key string)
func (m Metadata) Has(key string) bool

// 操作
func (m Metadata) Clone() Metadata
func (m Metadata) Merge(other Metadata)
func (m Metadata) ToMap() map[string]string
func FromMap(data map[string]string) Metadata
```

---

## 最佳实践

### 1. 使用构造函数

```go
// ✅ 推荐
req := protocol.NewRequest("Service", "Method", args)

// ❌ 不推荐（字段可能遗漏）
req := &protocol.Request{
    Service: "Service",
    Method:  "Method",
    // 忘记设置 ID、CreatedAt 等
}
```

### 2. 检查响应错误

```go
// ✅ 推荐
if resp.IsError() {
    // 处理错误
    code := resp.Error.Code
    if code == protocol.ErrorCodeNotFound {
        // 重试
    }
}

// ❌ 不推荐
if resp.Error != nil {  // 可能 panic（Error 是指针）
    // ...
}
```

### 3. 元数据的并发安全

```go
// ✅ 推荐（克隆）
req.Metadata = originalMeta.Clone()
// 修改 clone 不影响 original

// ❌ 不推荐（直接引用）
req.Metadata = originalMeta
// 多个 goroutine 可能并发修改
```

### 4. 超时设置

```go
// ✅ 推荐
req.SetTimeout(5 * time.Second)  // 语义清晰

// ❌ 不推荐
req.Timeout = 5000  // 需要知道单位是毫秒
```

---

## 测试覆盖

```
测试用例: 11 个
覆盖率:   56.7%
测试文件:
  - header_test.go
  - message_test.go
```

---

## 性能特点

```
Header 编码:   约 50 ns/op
Header 解码:   约 80 ns/op
Request 创建:  约 200 ns/op

所有操作都是纳秒级，性能优异！
```

---

## 扩展性

### 未来可扩展的点

1. **新的消息类型**：在 MessageType 中添加
2. **新的编解码**：在 CodecType 中添加
3. **新的压缩算法**：在 CompressType 中添加
4. **预留字段**：Header.Reserved 可用于扩展

---

## 依赖关系

```
Protocol 层:
  - 无外部依赖
  - 只依赖 Go 标准库
  
被依赖:
  - Codec 层（使用消息定义）
  - Transport 层（使用 Header）
```

---

**文档版本**: v1.0  
**最后更新**: 2026-01-02  
**作者**: Kunhua Huang





