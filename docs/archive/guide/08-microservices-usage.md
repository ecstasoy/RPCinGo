# 微服务使用指南

## 概述

本指南介绍如何在微服务项目中使用 RPCinGo 框架，包括服务注册、服务发现、负载均衡等核心功能。

---

## 快速开始

### 1. 定义 Proto 文件

首先定义服务的 Proto 文件：

```protobuf
// api/user/user.proto
syntax = "proto3";
package user;

option go_package = "your-project/api/user";

message GetUserRequest {
  int64 id = 1;
}

message GetUserResponse {
  int64 id = 1;
  string name = 2;
  string email = 3;
}

service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
}
```

生成代码：

```bash
protoc --go_out=. --go_opt=paths=source_relative api/user/user.proto
```

---

### 2. 实现服务提供者（Server）

**服务实现**：

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "your-project/api/user"
    "RPCinGo/pkg/protocol"
    "RPCinGo/pkg/registry/etcd"
    "RPCinGo/pkg/server"
)

type UserService struct{}

func (s *UserService) GetUser(ctx context.Context, req *user.GetUserRequest) (*user.GetUserResponse, error) {
    // 业务逻辑
    return &user.GetUserResponse{
        Id:    req.Id,
        Name:  "Alice",
        Email: "alice@example.com",
    }, nil
}

func main() {
    // 1. 连接 etcd
    etcdConfig := etcd.DefaultConfig()
    etcdConfig.Endpoints = []string{"localhost:2379"}
    
    etcdReg, err := etcd.NewEtcdRegistry(etcdConfig)
    if err != nil {
        fmt.Printf("connect etcd failed: %v\n", err)
        os.Exit(1)
    }
    defer etcdReg.Close()
    
    // 2. 创建 Server
    srv := server.NewServer(
        server.WithAddress("0.0.0.0:0"),  // 0 表示自动分配端口
        server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
        server.WithRegistry("UserService", "v1.0.0", etcdReg),
    )
    
    // 3. 注册服务实现
    userService := &UserService{}
    if err := srv.RegisterService("UserService", userService); err != nil {
        fmt.Printf("RegisterService failed: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Printf("UserService started on %s\n", srv.Addr())
    
    // 4. 启动服务
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    go func() {
        if err := srv.Start(ctx); err != nil {
            fmt.Printf("Server error: %v\n", err)
        }
    }()
    
    // 5. 优雅关闭
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    fmt.Println("\nShutting down...")
    srv.Stop()
    fmt.Println("Server stopped")
}
```

**关键点**：

- `WithRegistry`：注册服务到 Registry
- `RegisterService`：注册服务实现（自动识别强类型方法）
- `0.0.0.0:0`：自动分配端口
- `srv.Stop()`：自动注销服务

---

### 3. 实现服务消费者（Client）

**客户端代码**：

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "your-project/api/user"
    "RPCinGo/pkg/client"
    "RPCinGo/pkg/loadbalancer"
    "RPCinGo/pkg/protocol"
    "RPCinGo/pkg/registry/etcd"
)

func main() {
    // 1. 连接 etcd Discovery
    etcdConfig := etcd.DefaultConfig()
    etcdConfig.Endpoints = []string{"localhost:2379"}
    
    etcdDisc, err := etcd.NewEtcdDiscovery(etcdConfig)
    if err != nil {
        fmt.Printf("connect etcd failed: %v\n", err)
        os.Exit(1)
    }
    defer etcdDisc.Close()
    
    // 2. 创建 Discovery Client
    cli, err := client.NewDiscoveryClient(
        client.WithDiscovery(etcdDisc),
        client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
        client.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
        client.WithWatch(true),
        client.WithCircuitBreaker(true),
    )
    if err != nil {
        fmt.Printf("create client failed: %v\n", err)
        os.Exit(1)
    }
    defer cli.Close()
    
    // 3. 调用服务
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    req := &user.GetUserRequest{Id: 123}
    resp := &user.GetUserResponse{}
    
    if err := cli.CallTyped(ctx, "UserService", "GetUser", req, resp); err != nil {
        fmt.Printf("call failed: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Printf("User: ID=%d, Name=%s, Email=%s\n", resp.Id, resp.Name, resp.Email)
}
```

**关键点**：

- `NewDiscoveryClient`：使用服务发现的客户端
- `WithDiscovery`：配置服务发现
- `WithLoadBalancer`：配置负载均衡策略
- `WithWatch`：启用服务监听（自动更新实例列表）
- `CallTyped`：强类型调用

---

## 配置选项

### Server 配置

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithAddress` | 监听地址 | `:8080` |
| `WithCodec` | 编解码器类型 | `JSON` |
| `WithRegistry` | 服务注册 | 无 |
| `WithHeartbeatInterval` | 心跳间隔 | `5s` |

### Client 配置

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithDiscovery` | 服务发现 | 必需 |
| `WithLoadBalancer` | 负载均衡策略 | `RoundRobin` |
| `WithCodec` | 编解码器类型 | `JSON` |
| `WithWatch` | 启用服务监听 | `true` |
| `WithCircuitBreaker` | 启用熔断器 | `true` |
| `WithPoolSize` | 连接池大小 | `max=100, min=10` |
| `WithTimeout` | 调用超时 | `5s` |

---

## 负载均衡策略

框架提供三种负载均衡策略：

### 1. RoundRobin（轮询）

```go
client.WithLoadBalancer(loadbalancer.NewRoundRobin())
```

### 2. Random（随机）

```go
client.WithLoadBalancer(loadbalancer.NewRandom())
```

### 3. ConsistentHash（一致性哈希）

```go
client.WithLoadBalancer(loadbalancer.NewConsistentHash())
```

---

## 服务注册中心

### Memory Registry（测试用）

```go
import "RPCinGo/pkg/registry/memory"

memReg := memory.NewRegistry()
defer memReg.Close()

// Server
srv := server.NewServer(
    server.WithRegistry("UserService", "v1.0.0", memReg),
)

// Client
cli, _ := client.NewDiscoveryClient(
    client.WithDiscovery(memReg),
)
```

### Etcd Registry（生产环境）

```go
import "RPCinGo/pkg/registry/etcd"

config := etcd.DefaultConfig()
config.Endpoints = []string{"localhost:2379"}
config.DialTimeout = 5 * time.Second

// Server
etcdReg, _ := etcd.NewEtcdRegistry(config)
srv := server.NewServer(
    server.WithRegistry("UserService", "v1.0.0", etcdReg),
)

// Client
etcdDisc, _ := etcd.NewEtcdDiscovery(config)
cli, _ := client.NewDiscoveryClient(
    client.WithDiscovery(etcdDisc),
)
```

---

## 多实例部署

### 启动多个服务实例

```bash
# 实例 1
PORT=8080 go run services/user/main.go

# 实例 2
PORT=8081 go run services/user/main.go

# 实例 3
PORT=8082 go run services/user/main.go
```

客户端会自动：
- 发现所有实例
- 使用负载均衡选择实例
- 监听实例变化（新增/删除）
- 自动切换连接

---

## 编解码器选择

### JSON Codec（默认）

- 优点：易调试、跨语言兼容性好
- 缺点：性能较低、体积较大

```go
server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)
client.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeNone)
```

### Protobuf Codec（推荐）

- 优点：性能好、体积小、类型安全
- 缺点：需要生成代码

```go
server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone)
client.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone)
```

### 压缩选项

```go
// Gzip 压缩
server.WithCodec(protocol.CodecTypeJSON, protocol.CompressTypeGzip)
```

---

## 最佳实践

### 1. 使用 Protobuf

- 定义清晰的 Proto 文件
- 使用强类型方法（`RegisterService` + `CallTyped`）
- 获得类型安全和更好的性能

### 2. 生产环境配置

- 使用 etcd 作为 Registry
- 启用 Watch（自动发现服务变化）
- 启用 CircuitBreaker（保护下游服务）
- 配置合理的超时时间

### 3. 错误处理

```go
if err := cli.CallTyped(ctx, "UserService", "GetUser", req, resp); err != nil {
    // 检查错误类型
    if errors.Is(err, context.DeadlineExceeded) {
        // 超时处理
    } else {
        // 其他错误
    }
}
```

### 4. 服务版本管理

```go
// Server
server.WithRegistry("UserService", "v1.0.0", etcdReg)

// 升级到 v2.0.0
server.WithRegistry("UserService", "v2.0.0", etcdReg)
```

### 5. 环境变量配置

```go
etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
if etcdEndpoints == "" {
    etcdEndpoints = "localhost:2379"
}

config := etcd.DefaultConfig()
config.Endpoints = []string{etcdEndpoints}
```

---

## 故障排查

### 1. 服务无法注册

- 检查 etcd 连接
- 检查服务名称和版本
- 检查端口是否被占用

### 2. 客户端无法发现服务

- 检查 Discovery 配置
- 检查服务是否已注册
- 检查服务名称是否匹配

### 3. 调用超时

- 检查网络连接
- 检查服务是否正常运行
- 调整超时时间

### 4. 负载不均

- 检查负载均衡策略
- 检查实例健康状态
- 考虑使用一致性哈希

---

## 示例代码

完整示例代码请参考：

- `examples/microservice/` - 完整微服务示例

---

## 相关文档

- [架构概览](./00-architecture-overview.md)
- [Server 层](./06-rpc-server.md)
- [Client 层](./07-rpc-client.md)
- [Registry 层](./04-registry-layer.md)
- [Integration](./05-integration.md)




