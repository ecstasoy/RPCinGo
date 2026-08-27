# 快速开始

本文带你在 5 分钟内运行第一个 RPCinGo 调用。

## 前置要求

- Go 1.21+
- 已克隆本仓库

## 项目初始化

```bash
cd RPCinGo
go mod tidy
```

## 第一个 RPC 服务（JSON 模式）

### 1. 定义服务结构体

```go
// service.go
package main

import "context"

type HelloService struct{}

type HelloRequest struct {
    Name string `json:"name"`
}

type HelloResponse struct {
    Message string `json:"message"`
}

// 框架支持的方法签名（强类型形式）：
// func(ctx, *TypedReq) (*TypedResp, error)
func (s *HelloService) Greet(ctx context.Context,
    req *HelloRequest) (*HelloResponse, error) {
    return &HelloResponse{
        Message: "Hello, " + req.Name + "!",
    }, nil
}
```

### 2. 启动服务端

```go
// server/main.go
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
)

func main() {
    srv := server.NewServer(
        server.WithAddress("127.0.0.1:8080"),
        server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone),
        server.WithInterceptors(
            interceptor.NewRecoveryInterceptor(),
            interceptor.NewLoggingInterceptor(nil), // nil = 标准库 log
        ),
    )

    srv.RegisterService("Hello", &HelloService{})

    // 优雅退出
    go func() {
        quit := make(chan os.Signal, 1)
        signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
        <-quit
        srv.Stop()
    }()

    log.Println("Server listening on :8080")
    if err := srv.Start(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

### 3. 客户端调用

```go
// client/main.go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "RPCinGo/pkg/client"
)

func main() {
    cli, err := client.NewClient("127.0.0.1:8080",
        client.WithCallTimeout(5 * time.Second),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer cli.Close()

    ctx := context.Background()

    // 无类型调用（args 为 map）
    result, err := cli.Call(ctx, "Hello", "Greet",
        map[string]interface{}{"name": "World"})
    if err != nil {
        log.Fatal(err)
    }

    resp := result.(map[string]interface{})
    fmt.Println(resp["message"]) // Hello, World!
}
```

### 4. 运行

```bash
# 终端 1：服务端
go run server/main.go
# 输出: Server listening on :8080

# 终端 2：客户端
go run client/main.go
# 输出: Hello, World!
```

## 使用 Context 传递元数据

```go
import "RPCinGo/pkg/protocol"

// 客户端：创建带 Metadata 的 Request
ctx := context.Background()
// 通过 Metadata 传递 trace-id（通常由框架自动注入）
result, err := cli.CallWithMetadata(ctx, "Hello", "Greet",
    map[string]interface{}{"name": "Alice"},
    protocol.Metadata{
        protocol.MetadataKeyTraceID: "trace-abc-123",
    })
```

## 设置超时

```go
// 方式 1：配置全局调用超时
cli, _ := client.NewClient("127.0.0.1:8080",
    client.WithCallTimeout(3 * time.Second),
)

// 方式 2：单次调用超时（优先级更高）
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
defer cancel()
result, err := cli.Call(ctx, "Hello", "Greet", req)
if errors.Is(err, context.DeadlineExceeded) {
    log.Println("调用超时")
}
```

## 查看项目已有示例

```bash
# Calculator 示例（Protobuf 强类型）
go run examples/calculator/server/main.go &
go run examples/calculator/client/main.go

# 微服务示例（需要先启动 etcd）
docker run -d -p 2379:2379 quay.io/coreos/etcd:v3.5.0 \
    etcd --listen-client-urls http://0.0.0.0:2379 \
         --advertise-client-urls http://localhost:2379

go run examples/microservice/services/user/main.go &
go run examples/microservice/clients/user/main.go
```

## 下一步

- [Calculator 示例](calculator-example.md) — Protobuf 强类型调用（推荐生产方式）
- [微服务示例](microservice-example.md) — etcd 服务发现 + 负载均衡
- [Server 概述](../server/overview.md) — 服务端完整配置项
- [Client 概述](../client/overview.md) — 客户端完整配置项
- [拦截器链](../server/interceptors.md) — 添加日志/监控/限流
