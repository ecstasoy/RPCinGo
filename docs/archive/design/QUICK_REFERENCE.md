# Protobuf 强类型功能快速参考

## 🚀 快速开始

### Server 端

```go
// 1. 定义服务（使用强类型方法）
type CalculatorService struct{}

func (s *CalculatorService) Add(ctx context.Context, req *calculator.AddRequest) (*calculator.AddResponse, error) {
    return &calculator.AddResponse{Result: req.A + req.B}, nil
}

// 2. 注册服务（一行代码）
srv := server.NewServer(server.WithAddress("127.0.0.1:0"))
calcService := &CalculatorService{}
srv.RegisterService("Calculator", calcService)  // ✨ 自动注册所有方法
```

### Client 端

```go
// 强类型调用
cli, _ := client.NewClient("127.0.0.1:8080")
req := &calculator.AddRequest{A: 10, B: 20}
resp := &calculator.AddResponse{}

err := cli.CallTyped(ctx, "Calculator", "Add", req, resp)
sum := resp.Result  // ✅ 类型安全
```

---

## 📋 API 参考

### Server.RegisterService

```go
func (s *Server) RegisterService(serviceName string, serviceImpl interface{}) error
```

**参数**：
- `serviceName`: 服务名称
- `serviceImpl`: 服务实现（指针类型）

**方法签名要求**：
- `(ctx context.Context, req *Request) (*Response, error)`
- `Request` 和 `Response` 必须实现 `proto.Message`

---

### Client.CallTyped

```go
func (c *Client) CallTyped(ctx context.Context, service, method string, req proto.Message, resp proto.Message) error
```

**参数**：
- `ctx`: 上下文
- `service`: 服务名称
- `method`: 方法名称
- `req`: 请求消息（proto.Message）
- `resp`: 响应消息（proto.Message，用于接收结果）

**返回**：
- `error`: 错误信息

---

## 🔄 向后兼容

### 旧方式仍然可用

**Server**：
```go
srv.RegisterMethod("Calculator", "Add", func(ctx context.Context, args interface{}) (interface{}, error) {
    // ...
})
```

**Client**：
```go
result, err := client.Call(ctx, "Calculator", "Add", map[string]interface{}{"a": 10, "b": 20})
```

---

## 📝 完整示例

参考：
- `pkg/server/service_typed_test.go`
- `pkg/client/client_typed_test.go`
- `examples/proto/calculator/`

---

## 🎯 最佳实践

1. **使用强类型方法**：优先使用 `RegisterService` 和 `CallTyped`
2. **定义 Proto 文件**：为每个服务定义清晰的 Proto 消息
3. **类型安全**：利用 IDE 自动补全和编译时检查
4. **向后兼容**：旧代码可以逐步迁移

---

## ❓ 常见问题

### Q: 如何迁移现有代码？

A: 逐步迁移，新旧方法可以混合使用。

### Q: 性能影响？

A: 当前实现使用 JSON marshal/unmarshal，性能略有开销。未来可以优化。

### Q: 是否支持泛型？

A: 当前不支持，未来可以考虑使用 Go 1.18+ 泛型优化 API。

---

## 📚 相关文档

- `PROTOBUF_MIGRATION_PLAN.md` - 迁移计划
- `PROTOBUF_IMPLEMENTATION_SUMMARY.md` - Server 实现
- `CLIENT_TYPED_IMPLEMENTATION.md` - Client 实现
- `PROTOBUF_COMPLETE_SUMMARY.md` - 完整总结




