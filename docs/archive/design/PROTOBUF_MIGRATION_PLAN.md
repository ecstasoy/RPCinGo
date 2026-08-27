# Protobuf 迁移计划

## 📋 当前实现复盘

### 🔴 核心问题

#### 1. **类型不安全**

**当前设计**：
```go
// pkg/server/service.go
type MethodHandler func(ctx context.Context, args interface{}) (interface{}, error)

// pkg/protocol/request.go
type Request struct {
    Args interface{} `json:"args"`  // ❌ 弱类型
}

// pkg/protocol/response.go
type Response struct {
    Data interface{} `json:"data"`  // ❌ 弱类型
}
```

**业务代码示例**（当前方式）：
```go
func (s *UserService) GetUser(ctx context.Context, args interface{}) (interface{}, error) {
    // ❌ 手动类型转换
    m := args.(map[string]interface{})
    
    // ❌ 手动字段提取
    id, ok := m["user_id"]
    if !ok {
        return nil, errors.New("missing user_id")
    }
    
    // ❌ 手动类型断言
    userID := int(id.(float64))  // JSON unmarshal 数字默认是 float64
    
    user := s.users[userID]
    return user, nil
}
```

**问题总结**：
- ❌ 无编译时类型检查
- ❌ 运行时类型断言（容易 panic）
- ❌ 无 IDE 自动补全
- ❌ 手动参数验证
- ❌ JSON 数字默认 float64，需要手动转换

---

#### 2. **Protocol 层设计问题**

**当前 Codec 实现**：
```go
// pkg/codec/protobuf.go
func (c *ProtobufCodec) requestToProto(req *protocol.Request) (*pb.Request, error) {
    // ❌ 先用 JSON 序列化 args
    argsData, err := json.Marshal(req.Args)
    // ...
    pbReq.Args = argsData  // 字节数组，不是真正的 protobuf 消息
}
```

**问题**：
- Protobuf Codec 内部还在用 JSON 处理 Args
- `Args` 字段是 `[]byte`，不是类型化的 protobuf 消息
- 失去了 Protobuf 的类型安全优势

---

#### 3. **RegisterService 限制**

**当前实现**：
```go
// pkg/server/service.go:156
func methodKind(mt reflect.Type) (rpcMethodKind, bool) {
    emptyIface := reflect.TypeOf((*interface{})(nil)).Elem()
    
    // ❌ 要求必须是 interface{}
    if mt.In(2) != emptyIface {
        return 0, false
    }
    
    // ❌ 返回值也必须是 interface{}
    if mt.Out(0) != emptyIface {
        return 0, false
    }
}
```

**问题**：
- 只接受 `(ctx, interface{}) (interface{}, error)` 签名
- 不能注册 `(ctx, *GetUserRequest) (*GetUserResponse, error)` 这样的强类型方法

---

#### 4. **数据流问题**

**当前数据流**：
```
Client.Call(service, method, map[string]interface{}{...})
  ↓
protocol.Request{Args: interface{}}  // map 被序列化
  ↓
JSON/Protobuf 序列化 (Args 变成 []byte)
  ↓
网络传输
  ↓
Server 反序列化 (Args 变成 map[string]interface{})
  ↓
MethodHandler(ctx, args interface{})  // 需要手动转换
  ↓
业务方法手动类型断言
```

**问题**：
- 每个环节都需要类型转换
- 类型信息在序列化时丢失
- 无类型校验

---

## ✅ Protobuf 迁移方案

### 🎯 目标设计

**理想的方法签名**：
```go
// ✅ 强类型方法
func (s *UserService) GetUser(
    ctx context.Context,
    req *pb.GetUserRequest,
) (*pb.GetUserResponse, error) {
    // ✅ 直接使用 req.UserId，类型安全！
    user := s.users[req.UserId]
    return &pb.GetUserResponse{User: user}, nil
}
```

**数据流**：
```
Client.Call(ctx, service, method, &pb.GetUserRequest{UserId: 123})
  ↓
protocol.Request{Args: proto.Message}  // 类型化
  ↓
Protobuf 序列化 (类型信息保留)
  ↓
网络传输
  ↓
Server 反序列化到具体类型
  ↓
MethodHandler(ctx, *pb.GetUserRequest)  // 直接调用
  ↓
业务方法直接使用强类型
```

---

### 📐 架构设计

#### 方案选择：混合模式（兼容 + 强类型）

**为什么不用纯 Protobuf IDL**：
- 需要代码生成步骤（增加复杂度）
- 当前框架已经支持 JSON，需要保持兼容

**为什么不用纯反射 + 结构体**：
- 结构体没有版本控制
- 跨语言支持差
- 无向后兼容保证

**混合模式**：
- ✅ 支持 Protobuf 强类型（推荐）
- ✅ 保留 JSON 兼容性（向后兼容）
- ✅ 框架自动选择编解码方式

---

## 🔧 实施步骤

### Step 1: 定义 Protobuf 消息

为每个服务定义 `.proto` 文件：

```protobuf
// examples/microservices/proto/user/user.proto
syntax = "proto3";

package user;
option go_package = "RPCinGo/examples/microservices/proto/user";

message GetUserRequest {
  int32 user_id = 1;
}

message GetUserResponse {
  User user = 1;
}

message User {
  int32 id = 1;
  string name = 2;
  string email = 3;
}
```

---

### Step 2: 改进 Protocol 层

**方案 A：扩展 Request/Response（推荐）**

```go
type Request struct {
    ID             uint64
    Service        string
    Method         string
    ServiceVersion string
    Args           interface{}  // 保留兼容性
    ArgsType       string       // "json" | "protobuf"
    ArgsData       []byte       // 原始数据
    // ...
}

// 新增辅助方法
func (r *Request) GetArgsAsProto(msg proto.Message) error {
    // 从 ArgsData 反序列化到 msg
}

func (r *Request) SetArgsFromProto(msg proto.Message) error {
    // 序列化 msg 到 ArgsData
}
```

**方案 B：保持当前结构，改进 Codec**

Codec 层自动识别类型：
- 如果 Args 是 `proto.Message`，用 Protobuf 序列化
- 如果 Args 是其他类型，用 JSON 序列化

---

### Step 3: 改进 RegisterService

**支持两种方法签名**：

```go
// 模式 1：弱类型（兼容现有代码）
func (s *Service) OldMethod(ctx context.Context, args interface{}) (interface{}, error)

// 模式 2：强类型（新推荐）
func (s *Service) NewMethod(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error)
```

**改进 `methodKind`**：
```go
func methodKind(mt reflect.Type) (rpcMethodKind, bool) {
    // 检查是否是 proto.Message
    protoMsgType := reflect.TypeOf((*proto.Message)(nil)).Elem()
    
    // 模式 1: (ctx, interface{}) (interface{}, error)
    // 模式 2: (ctx, *Request) (*Response, error)  // Request/Response 是 proto.Message
}
```

---

### Step 4: 改进 Server Handler

**自动类型转换**：
```go
func (sr *ServiceRegistry) GetHandler(service, method string) (MethodHandler, error) {
    handler, ok := svc.GetMethod(method)
    if !ok {
        return nil, fmt.Errorf("method not found")
    }
    
    // 如果 handler 是强类型，包装成统一接口
    return wrapHandler(handler), nil
}

func wrapHandler(handler interface{}) MethodHandler {
    // 使用反射检查 handler 类型
    // 如果是强类型，自动编解码
    // 如果是弱类型，直接使用
}
```

---

### Step 5: 改进 Client

**支持强类型调用**：
```go
// 方式 1：当前方式（兼容）
client.Call(ctx, "UserService", "GetUser", map[string]interface{}{"user_id": 123})

// 方式 2：强类型（新推荐）
client.Call(ctx, "UserService", "GetUser", &pb.GetUserRequest{UserId: 123})
```

---

## 📝 实施优先级

### Phase 1: 核心改进（必须）
1. ✅ 定义 Protobuf 消息结构
2. ✅ 改进 Codec 支持 proto.Message
3. ✅ 改进 RegisterService 支持强类型方法
4. ✅ 改进 Server Handler 自动编解码

### Phase 2: 客户端改进（推荐）
5. ✅ 改进 Client.Call 支持强类型
6. ✅ 生成代码工具（可选）

### Phase 3: 工具链（可选）
7. ✅ Protoc 集成脚本
8. ✅ 代码生成工具

---

## 🎯 预期收益

### 类型安全
- ✅ 编译时类型检查
- ✅ IDE 自动补全
- ✅ 减少运行时错误

### 开发体验
- ✅ 代码更简洁
- ✅ 无需手动类型转换
- ✅ 自动参数验证

### 性能
- ✅ Protobuf 序列化更快
- ✅ 数据体积更小
- ✅ 减少 JSON 解析开销

### 兼容性
- ✅ 向后兼容现有 JSON 代码
- ✅ 渐进式迁移
- ✅ 双模式支持

---

## ⚠️ 注意事项

1. **向后兼容**：保持 `interface{}` 方式可用
2. **性能考虑**：反射有一定开销，考虑缓存
3. **错误处理**：类型不匹配时的友好错误
4. **文档**：清晰说明两种使用方式

---

## 📚 参考

- gRPC 服务定义方式
- Thrift IDL 设计
- Dubbo 服务接口定义




