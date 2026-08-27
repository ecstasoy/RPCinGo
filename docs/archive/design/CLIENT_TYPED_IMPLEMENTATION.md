# Client 层强类型调用实现总结

## ✅ 已完成的工作

### 1. 添加 CallTyped 方法

**文件**：`pkg/client/client.go`

#### 方法签名

```go
func (c *Client) CallTyped(ctx context.Context, service, method string, req proto.Message, resp proto.Message) error
```

#### 功能

- ✅ 接受 `proto.Message` 作为请求参数
- ✅ 将响应数据反序列化到 `proto.Message`
- ✅ 自动处理类型转换（JSON marshal/unmarshal）
- ✅ 错误处理

---

## 🎯 使用方式对比

### 之前（弱类型）

```go
result, err := client.Call(ctx, "Calculator", "Add",
    map[string]interface{}{"a": 10, "b": 20})

resultMap := result.(map[string]interface{})
sum := int(resultMap["result"].(float64))  // ❌ 手动类型转换
```

### 现在（强类型）✨

```go
req := &calculator.AddRequest{A: 10, B: 20}
resp := &calculator.AddResponse{}

err := client.CallTyped(ctx, "Calculator", "Add", req, resp)
if err != nil {
    return err
}

sum := resp.Result  // ✅ 类型安全，直接使用！
```

---

## 📋 实现细节

### 数据流

```
CallTyped(req proto.Message, resp proto.Message)
  ↓
Call(ctx, service, method, req)  // 内部处理序列化
  ↓
resp.Data (interface{}, 通常是 map[string]interface{})
  ↓
JSON Marshal/Unmarshal
  ↓
resp (proto.Message)  // 反序列化到目标类型
```

### 关键代码

```go
func (c *Client) CallTyped(ctx context.Context, service, method string, req proto.Message, resp proto.Message) error {
    data, err := c.Call(ctx, service, method, req)
    if err != nil {
        return err
    }

    dataBytes, err := json.Marshal(data)
    if err != nil {
        return fmt.Errorf("marshal response data: %w", err)
    }

    return json.Unmarshal(dataBytes, resp)
}
```

---

## 🧪 测试

**文件**：`pkg/client/client_typed_test.go`

### 测试场景

1. ✅ 调用强类型方法（Add）
2. ✅ 调用强类型方法（Subtract）
3. ✅ 验证响应数据正确性

### 运行测试

```bash
cd pkg/client
go test -run TestClient_CallTyped -v
```

---

## 🔄 向后兼容

当前实现**完全向后兼容**：

1. ✅ `Call` 方法仍然可用（弱类型方式）
2. ✅ `CallTyped` 是新增方法，不影响现有代码
3. ✅ 两种方式可以混合使用

---

## 📚 完整示例

```go
package main

import (
    "context"
    "RPCinGo/pkg/client"
    "RPCinGo/examples/proto/calculator"
)

func main() {
    cli, _ := client.NewClient("127.0.0.1:8080")
    defer cli.Close()

    ctx := context.Background()

    // 强类型调用
    req := &calculator.AddRequest{A: 10, B: 20}
    resp := &calculator.AddResponse{}

    err := cli.CallTyped(ctx, "Calculator", "Add", req, resp)
    if err != nil {
        panic(err)
    }

    println(resp.Result)  // 30
}
```

---

## 🚀 下一步工作

### 可选优化

1. **性能优化**：
   - 如果 codec 是 Protobuf，直接使用 proto.Unmarshal
   - 避免 JSON marshal/unmarshal 的开销

2. **类型推断**：
   - 使用泛型（Go 1.18+）简化 API
   - `CallTyped[Resp](ctx, service, method, req) (Resp, error)`

3. **错误处理**：
   - 更详细的错误信息
   - 类型不匹配的错误提示

---

## 📝 参考

- `pkg/client/client.go` - 核心实现
- `pkg/client/client_typed_test.go` - 测试代码
- `examples/proto/calculator/` - 示例 Proto 定义




