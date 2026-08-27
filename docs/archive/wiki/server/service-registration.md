# 服务注册

## 概述

`pkg/server/service.go`（311 行）实现了服务方法的注册与反射调用。框架支持三种方法签名，通过反射自动识别并适配。

**源码位置**：`pkg/server/service.go`

## ServiceRegistry 结构

```go
type ServiceRegistry struct {
    services map[string]*Service
    mu       sync.RWMutex
}

type Service struct {
    name    string
    methods map[string]*MethodInfo
}

type MethodInfo struct {
    handler MethodHandler
    reqType reflect.Type // 请求参数类型（用于反序列化）
}

// handler 的函数类型
type MethodHandler func(ctx context.Context, args interface{}) (interface{}, error)
```

## 三种合法方法签名

框架支持的方法签名形式（按优先级排序）：

### 签名 1：简单形式（无 context，仅 args）

```go
func (s *Service) Method(args interface{}) error
```

适用于不需要 context 或返回值的简单操作。

### 签名 2：标准 context 形式（无显式返回值）

```go
func (s *Service) Method(ctx context.Context, args interface{}) error
```

### 签名 3：强类型形式（推荐生产使用）

```go
func (s *Service) Method(ctx context.Context, req *TypedRequest) (*TypedResponse, error)
```

`*TypedRequest` 可以是任意指针类型（Go struct 或 proto.Message），框架通过反射自动检测并使用正确的反序列化方式：

- 如果 `*TypedRequest` 实现了 `proto.Message` → 使用 `proto.Unmarshal`
- 否则 → 使用 `json.Unmarshal`

## 反射注册（RegisterService）

```go
func (r *ServiceRegistry) RegisterService(name string, impl interface{}) error {
    t := reflect.TypeOf(impl)
    v := reflect.ValueOf(impl)

    svc := &Service{
        name:    name,
        methods: make(map[string]*MethodInfo),
    }

    for i := 0; i < t.NumMethod(); i++ {
        method := t.Method(i)
        if !method.IsExported() {
            continue // 跳过未导出方法
        }

        handler, reqType, ok := makeHandler(v, method)
        if !ok {
            // 签名不匹配，跳过（不报错）
            continue
        }

        svc.methods[method.Name] = &MethodInfo{
            handler: handler,
            reqType: reqType,
        }
    }

    r.mu.Lock()
    r.services[name] = svc
    r.mu.Unlock()
    return nil
}
```

### makeHandler 签名识别

```go
func makeHandler(v reflect.Value, method reflect.Method) (MethodHandler, reflect.Type, bool) {
    mt := method.Type

    // 签名 3：func(ctx, *TypedReq) (*TypedResp, error)
    if mt.NumIn() == 3 && mt.NumOut() == 2 {
        ctxType := mt.In(1)
        reqType := mt.In(2)
        if ctxType.Implements(contextInterface) &&
            reqType.Kind() == reflect.Ptr &&
            mt.Out(1).Implements(errorInterface) {

            handler := func(ctx context.Context, args interface{}) (interface{}, error) {
                // 反序列化 args 到 reqType
                req := reflect.New(reqType.Elem())
                if err := unmarshalArgs(args, req.Interface(), reqType); err != nil {
                    return nil, err
                }
                out := method.Func.Call([]reflect.Value{v, reflect.ValueOf(ctx), req})
                if !out[1].IsNil() {
                    return nil, out[1].Interface().(error)
                }
                return out[0].Interface(), nil
            }
            return handler, reqType, true
        }
    }

    // 签名 2：func(ctx, args) error
    // 签名 1：func(args) error
    // ... 类似处理
    return nil, nil, false
}
```

### 自动 Protobuf/JSON 检测

```go
func unmarshalArgs(args interface{}, req interface{}, reqType reflect.Type) error {
    // 检测是否为 proto.Message
    if protoMsg, ok := req.(proto.Message); ok {
        // args 应为 []byte（Protobuf 序列化结果）
        if data, ok := args.([]byte); ok {
            return proto.Unmarshal(data, protoMsg)
        }
    }
    // 否则用 JSON 反序列化
    data, err := json.Marshal(args) // 先转 JSON bytes（args 可能是 map）
    if err != nil {
        return err
    }
    return json.Unmarshal(data, req)
}
```

## 手动注册方法（RegisterMethod）

直接注册 handler 函数，适用于动态场景：

```go
srv.RegisterMethod("Calculator", "Multiply",
    func(ctx context.Context, args interface{}) (interface{}, error) {
        req := args.(*calculator.MultiplyRequest)
        return &calculator.MultiplyResponse{Result: req.A * req.B}, nil
    })
```

手动注册时框架不做类型检测，调用方自行做类型断言。

## 调用路由

收到请求后，`ServiceRegistry` 通过服务名和方法名查找 handler：

```go
func (r *ServiceRegistry) Invoke(ctx context.Context,
    req *protocol.Request) (interface{}, error) {

    r.mu.RLock()
    svc, ok := r.services[req.Service]
    r.mu.RUnlock()

    if !ok {
        return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, req.Service)
    }

    info, ok := svc.methods[req.Method]
    if !ok {
        return nil, fmt.Errorf("%w: %s.%s", ErrMethodNotFound, req.Service, req.Method)
    }

    return info.handler(ctx, req.Args)
}
```

路由键是 `Service.Method` 两级结构，而非扁平字符串，便于未来支持服务版本（`ServiceVersion`）路由。

## 完整示例

```go
// 三种签名都在同一个 service 中
type MixedService struct{}

// 签名 1：简单形式
func (s *MixedService) Ping(args interface{}) error {
    return nil
}

// 签名 2：带 context
func (s *MixedService) Health(ctx context.Context, args interface{}) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        return nil
    }
}

// 签名 3：强类型（推荐）
func (s *MixedService) GetInfo(ctx context.Context,
    req *InfoRequest) (*InfoResponse, error) {
    return &InfoResponse{Version: "1.0.0"}, nil
}

// 注册：三个方法全部自动注册
srv.RegisterService("Mixed", &MixedService{})
```

## 注意事项

- **方法名大小写敏感**：`GetUser` 和 `getUser` 是不同方法
- **同名覆盖**：`RegisterService` 多次调用同名服务，新注册覆盖旧注册
- **签名不匹配时静默跳过**：不符合三种签名的方法被忽略，不报错（注意检查日志）
- **未导出方法不注册**：`func (s *Service) privateMethod()` 不会被注册

## 相关文档

- [Server 概述](overview.md)
- [拦截器链](interceptors.md) — handler 调用前的拦截处理
- [Protobuf Codec](../codec/protobuf.md) — proto.Message 要求
