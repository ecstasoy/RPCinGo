# Calculator 示例

演示基于 Protobuf 的强类型 RPC 调用，是生产环境推荐的使用方式。完整代码位于 `examples/calculator/`。

## 目录结构

```
examples/calculator/
├── proto/
│   └── calculator.proto          ← 接口定义（如有）
├── server/
│   └── main.go                   ← 服务端（66 行）
└── client/
    └── main.go                   ← 客户端（53 行）
```

## 服务端实现

**源码**：`examples/calculator/server/main.go`

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "RPCinGo/pkg/interceptor"
    "RPCinGo/pkg/protocol"
    "RPCinGo/pkg/server"
    // Protobuf 生成的类型
    calcpb "RPCinGo/examples/calculator/proto"
)

// 服务实现
type CalculatorService struct{}

func (s *CalculatorService) Add(ctx context.Context,
    req *calcpb.AddRequest) (*calcpb.AddResponse, error) {
    return &calcpb.AddResponse{Result: req.A + req.B}, nil
}

func (s *CalculatorService) Subtract(ctx context.Context,
    req *calcpb.SubtractRequest) (*calcpb.SubtractResponse, error) {
    return &calcpb.SubtractResponse{Result: req.A - req.B}, nil
}

func main() {
    srv := server.NewServer(
        server.WithAddress("127.0.0.1:8080"),
        server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone),
        // 生产建议：使用 Protobuf
        // server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
        server.WithInterceptors(
            interceptor.NewRecoveryInterceptor(),
            interceptor.NewLoggingInterceptor(nil),
        ),
    )

    // 一行注册服务（反射自动发现 Add 和 Subtract 方法）
    if err := srv.RegisterService("Calculator", &CalculatorService{}); err != nil {
        log.Fatal(err)
    }

    // 优雅退出（来自实际源码）
    go func() {
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
        <-sigChan
        log.Println("Shutting down server...")
        srv.Stop()
    }()

    log.Println("Calculator server starting on :8080")
    if err := srv.Start(context.Background()); err != nil {
        log.Printf("Server stopped: %v", err)
    }
}
```

## 客户端实现

**源码**：`examples/calculator/client/main.go`

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "RPCinGo/pkg/client"
    calcpb "RPCinGo/examples/calculator/proto"
)

func main() {
    cli, err := client.NewClient("127.0.0.1:8080",
        client.WithCallTimeout(5 * time.Second),
    )
    if err != nil {
        log.Fatalf("Failed to create client: %v", err)
    }
    defer cli.Close()

    ctx := context.Background()

    // 强类型调用 Add
    addReq := &calcpb.AddRequest{A: 10, B: 20}
    addResp := &calcpb.AddResponse{}
    if err := cli.CallTyped(ctx, "Calculator", "Add", addReq, addResp); err != nil {
        log.Fatalf("Add failed: %v", err)
    }
    fmt.Printf("10 + 20 = %d\n", addResp.Result) // 10 + 20 = 30

    // 强类型调用 Subtract
    subReq := &calcpb.SubtractRequest{A: 100, B: 37}
    subResp := &calcpb.SubtractResponse{}
    if err := cli.CallTyped(ctx, "Calculator", "Subtract", subReq, subResp); err != nil {
        log.Fatalf("Subtract failed: %v", err)
    }
    fmt.Printf("100 - 37 = %d\n", subResp.Result) // 100 - 37 = 63
}
```

## 运行示例

```bash
# 生成 Protobuf 代码（如尚未生成）
bash scripts/gen-example-proto.sh

# 终端 1：启动服务端
go run examples/calculator/server/main.go
# 输出：Calculator server starting on :8080

# 终端 2：运行客户端
go run examples/calculator/client/main.go
# 输出：
# 10 + 20 = 30
# 100 - 37 = 63
```

## CallTyped 内部流程

```
cli.CallTyped(ctx, "Calculator", "Add", addReq, addResp)
    │
    ├── proto.Marshal(addReq)
    │   → []byte{0x08, 0x0A, 0x10, 0x14}（Protobuf 编码）
    │
    ├── 构建 Request{
    │     Service: "Calculator",
    │     Method:  "Add",
    │     Args:    []byte{...},  // 已序列化的 payload
    │     ArgsCodec: PROTOBUF,
    │   }
    │
    ├── 发送请求，等待响应
    │
    ├── 收到 Response{Data: []byte{...}}
    │
    └── proto.Unmarshal(Response.Data, addResp)
        addResp.Result = 30  ✅
```

## `CallTyped` vs `Call` 选择指南

| 维度 | `Call()` | `CallTyped()` |
|------|----------|---------------|
| 参数类型 | `interface{}`（map、struct 均可）| `proto.Message` |
| 返回处理 | 类型断言 `result.(map[string]interface{})` | 直接用 `resp` |
| 编译期检查 | ❌ 无 | ✅ 有 |
| 序列化 | JSON（默认）| Protobuf |
| 性能 | 中等 | 高（3–8x）|
| 适用场景 | 快速原型、动态类型 | **生产推荐** |

## 添加 Metrics 监控

```go
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithInterceptors(
        interceptor.NewRecoveryInterceptor(),
        interceptor.NewLoggingInterceptor(nil),
        interceptor.NewMetricsInterceptor(), // 添加 Prometheus 监控
    ),
)

// 暴露指标端点
go func() {
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9090", nil)
}()
```

## 相关文档

- [Protobuf Codec](../codec/protobuf.md) — CallTyped 内部使用的序列化
- [服务注册](../server/service-registration.md) — 方法签名与反射注册
- [微服务示例](microservice-example.md) — 服务发现 + 负载均衡
