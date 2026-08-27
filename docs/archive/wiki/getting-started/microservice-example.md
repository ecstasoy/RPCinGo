# 微服务示例

演示完整的微服务场景：etcd 服务注册、多实例部署、Round Robin 负载均衡、熔断器、实时 Watch。完整代码位于 `examples/microservice/`。

## 目录结构

```
examples/microservice/
├── services/user/
│   └── main.go          ← UserService 服务端（135 行）
└── clients/user/
    └── main.go          ← 客户端（81 行）
```

## 前置准备

### 启动 etcd

```bash
# Docker 方式（推荐）
docker run -d --name etcd-dev \
    -p 2379:2379 \
    quay.io/coreos/etcd:v3.5.0 \
    etcd \
    --listen-client-urls http://0.0.0.0:2379 \
    --advertise-client-urls http://localhost:2379

# 验证启动
docker exec etcd-dev etcdctl endpoint health
```

## UserService 服务端

**源码**：`examples/microservice/services/user/main.go`（135 行）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "strconv"
    "syscall"

    "RPCinGo/pkg/interceptor"
    "RPCinGo/pkg/protocol"
    "RPCinGo/pkg/registry/etcd"
    "RPCinGo/pkg/server"
    userpb "RPCinGo/examples/microservice/proto"
)

// 模拟内存数据库
var users = map[int64]*userpb.User{
    1: {Id: 1, Name: "Alice", Email: "alice@example.com"},
    2: {Id: 2, Name: "Bob",   Email: "bob@example.com"},
    3: {Id: 3, Name: "Carol", Email: "carol@example.com"},
}

type UserService struct {
    instanceID string // 用于追踪是哪个实例处理的请求
}

func (s *UserService) GetUser(ctx context.Context,
    req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
    user, ok := users[req.UserId]
    if !ok {
        return nil, fmt.Errorf("user %d not found", req.UserId)
    }
    return &userpb.GetUserResponse{
        User:       user,
        InstanceId: s.instanceID, // 让客户端知道是哪个实例响应的
    }, nil
}

func (s *UserService) ListUsers(ctx context.Context,
    req *userpb.ListUsersRequest) (*userpb.ListUsersResponse, error) {
    list := make([]*userpb.User, 0, len(users))
    for _, u := range users {
        list = append(list, u)
    }
    return &userpb.ListUsersResponse{Users: list}, nil
}

func main() {
    // 从环境变量获取地址，支持多实例部署
    addr := os.Getenv("SERVER_ADDR")
    if addr == "" {
        addr = "127.0.0.1:8080"
    }

    // 从地址中提取端口作为实例 ID
    instanceID := addr

    // 初始化 etcd 注册中心
    etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
    if etcdEndpoints == "" {
        etcdEndpoints = "localhost:2379"
    }
    reg, err := etcd.NewRegistry(
        etcd.WithEndpoints(etcdEndpoints),
        etcd.WithLeaseTTL(30),
    )
    if err != nil {
        log.Fatalf("Failed to create registry: %v", err)
    }

    // 创建服务端（Protobuf + Gzip）
    srv := server.NewServer(
        server.WithAddress(addr),
        server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip),
        server.WithRegistry(reg, "UserService"),
        server.WithHeartbeatInterval(10),
        server.WithInterceptors(
            interceptor.NewRecoveryInterceptor(),
            interceptor.NewLoggingInterceptor(nil),
            interceptor.NewMetricsInterceptor(),
        ),
    )

    srv.RegisterService("UserService", &UserService{instanceID: instanceID})

    // 优雅退出
    go func() {
        sig := make(chan os.Signal, 1)
        signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
        <-sig
        log.Printf("[%s] Shutting down...", instanceID)
        srv.Stop()
    }()

    log.Printf("UserService starting on %s", addr)
    if err := srv.Start(context.Background()); err != nil {
        log.Printf("Server stopped: %v", err)
    }
}
```

## 多实例部署

```bash
# 在不同终端分别启动三个实例
SERVER_ADDR=127.0.0.1:8081 go run examples/microservice/services/user/main.go
SERVER_ADDR=127.0.0.1:8082 go run examples/microservice/services/user/main.go
SERVER_ADDR=127.0.0.1:8083 go run examples/microservice/services/user/main.go
```

## 客户端（服务发现 + 负载均衡）

**源码**：`examples/microservice/clients/user/main.go`（81 行）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "RPCinGo/pkg/client"
    "RPCinGo/pkg/loadbalancer"
    "RPCinGo/pkg/registry/etcd"
    userpb "RPCinGo/examples/microservice/proto"
)

func main() {
    // 初始化服务发现
    disc, err := etcd.NewDiscovery(
        etcd.WithEndpoints("localhost:2379"),
        etcd.WithDialTimeout(5 * time.Second),
    )
    if err != nil {
        log.Fatalf("Failed to create discovery: %v", err)
    }

    // 创建 Discovery 模式客户端
    cli, err := client.NewDiscoveryClient(
        client.WithDiscovery(disc),
        client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
        client.WithCircuitBreaker(true),
        client.WithCallTimeout(5 * time.Second),
        client.WithWatch(true),        // 启用后台 Watch，实时感知实例变化
        client.WithMaxConnections(20), // 每个实例 20 个连接
    )
    if err != nil {
        log.Fatalf("Failed to create client: %v", err)
    }
    defer cli.Close()

    ctx := context.Background()

    // 连续调用 9 次，观察轮询负载均衡效果（3 实例 × 3 轮）
    fmt.Println("=== GetUser（轮询 3 实例）===")
    for i := int64(1); i <= 3; i++ {
        for round := 0; round < 3; round++ {
            req := &userpb.GetUserRequest{UserId: i}
            resp := &userpb.GetUserResponse{}
            if err := cli.CallTyped(ctx, "UserService", "GetUser", req, resp); err != nil {
                log.Printf("GetUser(%d) error: %v", i, err)
                continue
            }
            fmt.Printf("User %d: %s [by %s]\n",
                i, resp.User.Name, resp.InstanceId)
        }
    }

    // 调用 ListUsers
    fmt.Println("\n=== ListUsers ===")
    listReq := &userpb.ListUsersRequest{}
    listResp := &userpb.ListUsersResponse{}
    if err := cli.CallTyped(ctx, "UserService", "ListUsers", listReq, listResp); err != nil {
        log.Fatalf("ListUsers error: %v", err)
    }
    for _, user := range listResp.Users {
        fmt.Printf("  - %d: %s (%s)\n", user.Id, user.Name, user.Email)
    }
}
```

## 预期输出

```
=== GetUser（轮询 3 实例）===
User 1: Alice [by 127.0.0.1:8081]
User 1: Alice [by 127.0.0.1:8082]
User 1: Alice [by 127.0.0.1:8083]
User 2: Bob   [by 127.0.0.1:8081]
User 2: Bob   [by 127.0.0.1:8082]
User 2: Bob   [by 127.0.0.1:8083]
User 3: Carol [by 127.0.0.1:8081]
User 3: Carol [by 127.0.0.1:8082]
User 3: Carol [by 127.0.0.1:8083]

=== ListUsers ===
  - 1: Alice (alice@example.com)
  - 2: Bob (bob@example.com)
  - 3: Carol (carol@example.com)
```

请求按 8081 → 8082 → 8083 → 8081 ... 循环分配，验证 Round Robin 生效。

## 动态扩缩容测试

客户端持续发送请求时，动态增减服务实例，验证 Watch 机制：

```bash
# 停止 8082 实例（Ctrl+C）
# 客户端自动感知（Watch DELETE 事件），后续请求只路由到 8081 和 8083

# 启动新实例 8084
SERVER_ADDR=127.0.0.1:8084 go run examples/microservice/services/user/main.go
# 客户端自动感知（Watch ADD 事件），8084 立即参与负载均衡
```

## 配置解析：Protobuf + Gzip

本示例使用 `CodecTypeProtobuf + CompressTypeGzip`，适合包含大量用户列表等大响应的场景：

```
编码：proto.Marshal(ListUsersResponse{...}) → 100B
压缩：gzip.Compress(100B) → 60B（压缩率 40%）
传输：60B（节省 40% 带宽）
```

对于 `GetUser` 这类小消息（< 50B），Gzip 可能适得其反（压缩后反而更大），可改为 `CompressTypeNone`。

## etcd 验证注册情况

```bash
# 查看已注册的 UserService 实例
docker exec etcd-dev etcdctl get /rpc/services/UserService --prefix

# 输出示例：
# /rpc/services/UserService/127.0.0.1:8081
# {"id":"127.0.0.1:8081","service":"UserService","address":"127.0.0.1","port":8081,"status":1,...}
```

## 相关文档

- [etcd 注册中心](../registry/etcd.md) — 注册机制详解
- [服务发现模式](../client/discovery-mode.md) — Discovery Client 完整配置
- [负载均衡算法](../loadbalancer/algorithms.md) — Round Robin 实现
- [熔断器](../reliability/circuit-breaker.md) — 每实例熔断器
