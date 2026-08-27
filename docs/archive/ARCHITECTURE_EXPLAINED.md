# RPC 框架架构详解

## 📚 目录

1. [整体架构层次](#整体架构层次)
2. [核心概念](#核心概念)
3. [数据流详解](#数据流详解)
4. [关键设计决策](#关键设计决策)
5. [示例说明](#示例说明)

---

## 整体架构层次

```
┌─────────────────────────────────────────────────────────┐
│                   应用层 (Application)                    │
│  client.Call() / server.RegisterMethod() / CallTyped()   │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│                  RPC 层 (RPC Layer)                      │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   Client         │      │   Server         │         │
│  │   - Call()       │◄────►│   - Handle()     │         │
│  │   - CallTyped()  │      │   - Register()   │         │
│  └────────┬─────────┘      └────────┬─────────┘         │
└───────────┼──────────────────────────┼──────────────────┘
            │                          │
┌───────────▼──────────────────────────▼──────────────────┐
│              协议层 (Protocol Layer)                     │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   Request        │      │   Response       │         │
│  │   - Args         │      │   - Data         │         │
│  │   - ArgsCodec    │      │   - DataCodec    │         │
│  └──────────────────┘      └──────────────────┘         │
└───────────┼──────────────────────────┼──────────────────┘
            │                          │
┌───────────▼──────────────────────────▼──────────────────┐
│           编解码层 (Codec Layer)                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   JSONCodec      │      │   ProtobufCodec  │         │
│  │   MsgPackCodec   │      │   ...            │         │
│  └──────────────────┘      └──────────────────┘         │
└───────────┼──────────────────────────┼──────────────────┘
            │                          │
┌───────────▼──────────────────────────▼──────────────────┐
│           传输层 (Transport Layer)                       │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   TCP Client     │      │   TCP Server     │         │
│  │   - SendRequest()│◄────►│   - Serve()      │         │
│  └──────────────────┘      └──────────────────┘         │
└─────────────────────────────────────────────────────────┘
```

---

## 核心概念

### 1. 分层职责

#### 应用层
- **Client**: 用户调用的 API，如 `client.Call("Service", "Method", args)`
- **Server**: 服务注册和方法处理

#### RPC 层
- **Client**: 管理连接池、负载均衡、熔断、重试
- **Server**: 服务注册表、方法路由、拦截器链

#### 协议层
- **Request/Response**: 定义 RPC 消息结构
- **关键字段**:
  - `Args`: 请求参数（类型：`interface{}`，实际：`[]byte` 或原始类型）
  - `ArgsCodec`: 参数编码类型（`PayloadCodec`）
  - `Data`: 响应数据（类型：`interface{}`，实际：`[]byte` 或原始类型）
  - `DataCodec`: 数据编码类型（`PayloadCodec`）

#### 编解码层
- **Codec**: 负责将 `Request/Response` 结构序列化为字节流
- **特殊处理**: 对于 `Request/Response`，需要将 `Args/Data` 编码为 `[]byte` 并设置 `Codec` 字段

#### 传输层
- **TCP Client/Server**: 负责网络 I/O，发送/接收字节流

### 2. PayloadCodec（载荷编码类型）

```go
type PayloadCodec int32

const (
    PayloadCodecUnknown  = 0  // 未知，需要推断
    PayloadCodecRaw      = 1  // 原始字节
    PayloadCodecJSON     = 2  // JSON
    PayloadCodecProtobuf = 3  // Protobuf
)
```

**为什么需要 `ArgsCodec` 和 `DataCodec`？**

1. **明确编码类型**: 不再猜测，避免错误
2. **支持多种序列化**: JSON、Protobuf、MsgPack 等
3. **向后兼容**: 即使 `ArgsCodec` 是 `UNKNOWN`，也能推断

### 3. Args/Data 字段的类型变化

#### 在应用层（用户代码）
```go
// 用户传入的是原始类型
client.Call("Calculator", "Add", map[string]interface{}{"a": 10, "b": 20})
// 或
client.CallTyped("UserService", "GetUser", &GetUserRequest{Id: 1}, &GetUserResponse{})
```

#### 在协议层（Request/Response 结构）
```go
// 创建时，Args 是原始类型
req := &Request{
    Args: map[string]interface{}{"a": 10, "b": 20},  // 原始类型
}

// 经过 Codec 编码后，Args 变成 []byte
// 在网络上传输时，Args 始终是 []byte
req.Args = []byte(`{"a":10,"b":20}`)  // 编码后的字节
req.ArgsCodec = PayloadCodecJSON      // 标记编码类型
```

#### 在服务端 Handler
```go
// 接收到的 Args 是 []byte
func handler(ctx context.Context, req *protocol.Request) (interface{}, error) {
    argsBytes := req.Args.([]byte)  // 从 []byte 解码
    // 根据 ArgsCodec 决定如何解码
}
```

---

## 数据流详解

### 场景 1: 客户端调用（非强类型）

```
用户调用
  ↓
client.Call("Calculator", "Add", map[string]interface{}{"a": 10, "b": 20})
  ↓
【Client 层】创建 Request
  Request{
    Args: map[string]interface{}{"a": 10, "b": 20},  // 原始类型
    ArgsCodec: UNKNOWN,  // 未设置
  }
  ↓
【Codec 层】JSONCodec.Encode()
  - 检测到 Args 是 map[string]interface{}
  - 将 Args 编码为 []byte: []byte(`{"a":10,"b":20}`)
  - 设置 ArgsCodec = JSON
  - 序列化整个 Request 结构
  ↓
【传输层】TCP Client.SendRequest()
  - 添加 Header（Magic、Version、BodyLen 等）
  - 发送字节流到网络
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
                             网络传输
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
【传输层】TCP Server.handleConnection()
  - 读取 Header
  - 读取 Body（字节流）
  ↓
【Codec 层】JSONCodec.Decode()
  - 解码字节流为 Request 结构
  - Args 字段保持为 []byte
  - ArgsCodec = JSON
  ↓
【Server 层】Server.HandleRequest()
  - 找到对应的 Handler
  - 调用 Handler(ctx, req)
  ↓
【应用层】用户 Handler
  func handler(ctx context.Context, req *protocol.Request) (interface{}, error) {
      argsBytes := req.Args.([]byte)
      var argsMap map[string]interface{}
      json.Unmarshal(argsBytes, &argsMap)  // 手动解码
      a := int(argsMap["a"].(float64))
      b := int(argsMap["b"].(float64))
      return a + b, nil  // 返回原始类型
  }
  ↓
【Server 层】创建 Response
  Response{
    Data: 30,  // 原始类型
    DataCodec: UNKNOWN,  // 未设置
  }
  ↓
【Codec 层】JSONCodec.Encode()
  - 检测到 Data 是 int
  - 将 Data 编码为 []byte: []byte(`30`)
  - 设置 DataCodec = JSON
  - 序列化整个 Response 结构
  ↓
【传输层】TCP Server.WriteResponse()
  - 添加 Header
  - 发送字节流到网络
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
                             网络传输
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
【传输层】TCP Client.Send()
  - 读取 Header
  - 读取 Body
  ↓
【Codec 层】JSONCodec.Decode()
  - 解码字节流为 Response 结构
  - Data 字段保持为 []byte
  - DataCodec = JSON
  ↓
【Client 层】client.Call() 返回
  return resp, nil  // 返回 *protocol.Response
  ↓
【应用层】用户代码
  resp, err := client.Call(...)  // 直接返回 *protocol.Response
  dataBytes := resp.Data.([]byte)
  var sum int
  json.Unmarshal(dataBytes, &sum)  // 手动解码
```

### 场景 2: 客户端调用（强类型 CallTyped）

```
用户调用
  ↓
client.CallTyped("UserService", "GetUser", 
    &GetUserRequest{Id: 1}, 
    &GetUserResponse{})
  ↓
【Client 层】CallTyped()
  - 调用 Call() 获取 *protocol.Response
  - 从 resp.Data ([]byte) 解码到 resp proto.Message
  - 根据 DataCodec 选择解码方式（Protobuf 或 JSON）
  ↓
【应用层】用户直接使用强类型
  resp := &GetUserResponse{}
  client.CallTyped(..., req, resp)
  // resp 已经被填充了数据
```

### 场景 3: 服务端注册（强类型 RegisterService）

```
用户注册
  ↓
server.RegisterService("Calculator", &CalculatorService{})
  ↓
【Server 层】扫描服务的方法
  - 找到 Add(ctx, *AddRequest) (*AddResponse, error)
  - 识别为强类型方法（第三个参数是指针，返回值是指针）
  ↓
【Server 层】创建 Handler 包装器
  func handler(ctx context.Context, req *protocol.Request) (interface{}, error) {
      // 1. 从 req.Args ([]byte) 解码到 *AddRequest
      addReq := &AddRequest{}
      switch req.ArgsCodec {
      case PayloadCodecProtobuf:
          proto.Unmarshal(req.Args.([]byte), addReq)
      case PayloadCodecJSON:
          json.Unmarshal(req.Args.([]byte), addReq)
      }
      
      // 2. 调用真实方法
      addResp, err := service.Add(ctx, addReq)
      
      // 3. 返回结果（原始类型，后续会被 Codec 编码）
      return addResp, err
  }
  ↓
【Server 层】HandleRequest()
  - 调用 handler
  - 得到 addResp (*AddResponse)
  - 创建 Response{Data: addResp, DataCodec: UNKNOWN}
  ↓
【Codec 层】检测到 Data 是 proto.Message
  - 将 Data 编码为 []byte（Protobuf 编码）
  - 设置 DataCodec = Protobuf
```

---

## 关键设计决策

### 1. 为什么 Args/Data 在网络上总是 []byte？

**原因**:
- **一致性**: 统一处理，无论是 JSON、Protobuf 还是 MsgPack
- **灵活性**: 支持多种序列化格式
- **清晰性**: 明确知道数据是序列化后的字节

**缺点**:
- 需要额外的编码/解码步骤
- 用户需要手动解码（非强类型场景）

### 2. 为什么需要 ArgsCodec/DataCodec？

**原因**:
- **避免猜测**: 不再需要根据内容推断格式
- **类型安全**: 明确知道如何解码
- **扩展性**: 未来可以添加更多序列化格式（MsgPack、Avro 等）

### 3. 为什么在应用层 Args/Data 可以是原始类型？

**原因**:
- **用户体验**: 用户传入原始类型更自然
- **向后兼容**: 旧的代码不需要修改
- **灵活性**: Codec 会自动检测并编码

**处理流程**:
1. 用户传入原始类型 → Codec 检测类型 → 编码为 []byte → 设置 Codec
2. 用户传入 []byte → Codec 直接使用 → 设置 Codec（如果已知）

### 4. 强类型 vs 非强类型

#### 非强类型（RegisterMethod）
```go
server.RegisterMethod("Calculator", "Add", func(ctx context.Context, req *protocol.Request) (interface{}, error) {
    // 需要手动处理 req.Args ([]byte)
    argsBytes := req.Args.([]byte)
    var argsMap map[string]interface{}
    json.Unmarshal(argsBytes, &argsMap)
    // ... 处理逻辑
    return result, nil  // 返回原始类型
})
```

#### 强类型（RegisterService）
```go
type CalculatorService struct{}

func (s *CalculatorService) Add(ctx context.Context, req *AddRequest) (*AddResponse, error) {
    // req 已经是强类型 *AddRequest
    // 框架自动处理了 []byte → *AddRequest 的转换
    return &AddResponse{Result: req.A + req.B}, nil
}

server.RegisterService("Calculator", &CalculatorService{})
```

**优势**:
- 类型安全
- 代码更简洁
- IDE 自动补全
- 编译期检查

---

## 示例说明

### 完整示例：JSON 编码的 RPC 调用

#### 1. 客户端调用

```go
// 用户代码
client, _ := client.NewClient("127.0.0.1:8080")
resp, err := client.Call(ctx, "Calculator", "Add", map[string]interface{}{
    "a": 10,
    "b": 20,
})
```

#### 2. Client 层创建 Request

```go
// pkg/client/client.go
req := protocol.NewRequest("Calculator", "Add", map[string]interface{}{
    "a": 10,
    "b": 20,
})
// req.Args = map[string]interface{}{"a": 10, "b": 20}
// req.ArgsCodec = UNKNOWN
```

#### 3. Codec 层编码

```go
// pkg/codec/json.go - Encode()
if req, ok := v.(*protocol.Request); ok {
    // 检测到 Args 是 map[string]interface{}
    argsBytes, _ := json.Marshal(req.Args)  // []byte(`{"a":10,"b":20}`)
    
    // 创建临时结构，Args 字段设置为 []byte
    tempReq := struct {
        *protocol.Request
        Args      []byte `json:"args"`
        ArgsCodec PayloadCodec `json:"args_codec"`
    }{
        Request:   req,
        Args:      argsBytes,
        ArgsCodec: PayloadCodecJSON,
    }
    
    // 序列化整个结构
    return json.Marshal(tempReq)
}
```

#### 4. 网络传输

```go
// pkg/transport/tcp/codec.go
// 添加 Header
header := &protocol.Header{
    Magic:     0xCAFE,
    BodyLength: len(bodyBytes),
    // ...
}

// 发送: [Header(16 bytes)][Body(JSON bytes)]
```

#### 5. 服务端接收

```go
// pkg/transport/tcp/server.go
header, req, err := s.codec.ReadRequest(conn)
// req.Args = []byte(`{"a":10,"b":20}`)
// req.ArgsCodec = PayloadCodecJSON
```

#### 6. Server 层处理

```go
// pkg/server/server.go
handler, _ := s.registry.GetHandler("Calculator", "Add")
result, err := handler(ctx, req)  // 调用用户 Handler
// result = 30 (int)
```

#### 7. 用户 Handler

```go
// 用户代码
func handler(ctx context.Context, req *protocol.Request) (interface{}, error) {
    argsBytes := req.Args.([]byte)  // []byte(`{"a":10,"b":20}`)
    var argsMap map[string]interface{}
    json.Unmarshal(argsBytes, &argsMap)
    
    a := int(argsMap["a"].(float64))  // 10
    b := int(argsMap["b"].(float64))  // 20
    return a + b, nil  // 30
}
```

#### 8. 创建 Response

```go
// pkg/server/server.go
resp := protocol.NewSuccessResponse(req.ID, result)
// resp.Data = 30 (int)
// resp.DataCodec = UNKNOWN
```

#### 9. Codec 层编码 Response

```go
// pkg/codec/json.go - Encode()
if resp, ok := v.(*protocol.Response); ok {
    // 检测到 Data 是 int
    dataBytes, _ := json.Marshal(resp.Data)  // []byte(`30`)
    
    tempResp := struct {
        *protocol.Response
        Data      []byte `json:"data"`
        DataCodec PayloadCodec `json:"data_codec"`
    }{
        Response:  resp,
        Data:      dataBytes,
        DataCodec: PayloadCodecJSON,
    }
    
    return json.Marshal(tempResp)
}
```

#### 10. 客户端接收

```go
// pkg/client/client.go
resp, err := client.Call(...)
// resp.Data = []byte(`30`)
// resp.DataCodec = PayloadCodecJSON

// 用户需要手动解码
dataBytes := resp.Data.([]byte)
var sum int
json.Unmarshal(dataBytes, &sum)  // sum = 30
```

---

## 总结

### 核心原则

1. **网络上传输的 Args/Data 始终是 []byte**
2. **ArgsCodec/DataCodec 明确标记编码类型**
3. **应用层可以使用原始类型（由 Codec 自动转换）**
4. **强类型方法自动处理编码/解码**

### 数据转换流程

```
应用层（原始类型）
  ↓ Codec.Encode()
协议层（[]byte + Codec 标记）
  ↓ 网络传输
协议层（[]byte + Codec 标记）
  ↓ Codec.Decode() 或强类型方法自动解码
应用层（原始类型或强类型）
```

### 关键文件

- `pkg/protocol/request.go`: Request 结构定义
- `pkg/protocol/response.go`: Response 结构定义
- `pkg/codec/json.go`: JSON 编解码（特殊处理 Request/Response）
- `pkg/codec/protobuf.go`: Protobuf 编解码（特殊处理 Request/Response）
- `pkg/server/service.go`: 强类型方法处理（callTypedMethod）
- `pkg/client/client.go`: 客户端调用逻辑
- `pkg/transport/tcp/codec.go`: TCP 协议编解码（Header + Body）

---

**希望这个文档能帮助您理解整个架构！如果还有不清楚的地方，请告诉我具体是哪一部分。**

