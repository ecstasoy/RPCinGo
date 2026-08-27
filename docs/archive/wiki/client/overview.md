# Client 概述

## 职责

`pkg/client` 提供 RPC 客户端实现，支持两种运行模式、两种调用接口，以及连接池、服务发现、负载均衡、熔断和拦截器等生产级特性。

**源码位置**：`pkg/client/client.go`（351 行）、`pkg/client/options.go`（109 行）

## 核心结构

```go
type Client struct {
    opts clientOptions

    // Fixed 模式
    pool *pool.ConnectionPool

    // Discovery 模式
    discovery   registry.Discovery
    balancer    loadbalancer.LoadBalancer
    poolManager *pool.PoolManager
    breakers    sync.Map // map[address]*circuitbreaker.CircuitBreaker
    watcher     registry.Watcher  // 监听服务实例变化
    instances   []*registry.ServiceInstance
    instancesMu sync.RWMutex
}
```

## 两种运行模式

### Fixed 模式

直接连接到固定地址，适合开发/测试/单节点场景：

```go
cli, err := client.NewClient("127.0.0.1:8080",
    client.WithCodec(protocol.CodecTypeJSON),
    client.WithCompress(protocol.CompressTypeNone),
    client.WithCallTimeout(5 * time.Second),
    client.WithMaxConnections(100),
    client.WithMinConnections(10),
)
```

内部调用路径：`pool.Get() → tcp.Send() → tcp.Receive() → pool.Put()`

### Discovery 模式

通过注册中心动态发现服务实例，适合微服务生产环境：

```go
disc, _ := etcd.NewDiscovery(etcd.WithEndpoints("localhost:2379"))

cli, err := client.NewDiscoveryClient(
    client.WithDiscovery(disc),
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
    client.WithCircuitBreaker(true),
    client.WithCallTimeout(5 * time.Second),
    client.WithWatch(true),       // 启用后台 Watch 实时更新实例列表
)
```

内部调用路径：`GetInstances() → Pick() → CircuitBreaker.Allow() → PoolManager.GetConnection() → tcp.Send/Receive()`

## 两种调用接口

### Call()（无类型调用）

```go
func (c *Client) Call(ctx context.Context,
    service, method string,
    args interface{}) (interface{}, error)
```

- `args` 为任意 Go 对象，由 Codec 序列化
- 返回 `interface{}`，调用方需做类型断言
- 适合动态类型场景

```go
result, err := cli.Call(ctx, "UserService", "GetUser",
    map[string]interface{}{"user_id": 1001})
if err != nil { ... }
user := result.(map[string]interface{})
fmt.Println(user["name"])
```

### CallTyped()（强类型调用，推荐）

```go
func (c *Client) CallTyped(ctx context.Context,
    service, method string,
    req proto.Message,
    resp proto.Message) (*protocol.Response, error)
```

- `req` 和 `resp` 均为 Protobuf 生成类型
- 框架内部自动处理 `proto.Marshal`/`proto.Unmarshal`
- 编译期类型安全，无运行时断言
- 返回完整的 `*protocol.Response`，可读取 Response Metadata（如服务端 SpanID）

```go
req := &userpb.GetUserRequest{UserId: 1001}
resp := &userpb.GetUserResponse{}
rpcResp, err := cli.CallTyped(ctx, "UserService", "GetUser", req, resp)
if err != nil {
    log.Fatal(err)
}
fmt.Println(resp.Name)

// 读取服务端返回的追踪信息
if spanID, ok := rpcResp.GetMetadata("span-id"); ok {
    fmt.Println("Server SpanID:", spanID)
}
```

## Watch 机制

当 `WithWatch(true)` 时，Discovery 模式客户端会在后台维护一个 goroutine，持续监听 etcd 服务实例变化并更新本地缓存：

```go
// 后台 Watch goroutine（在 NewDiscoveryClient 内启动）
go func() {
    watcher, _ := c.discovery.Watch(ctx, serviceName)
    defer watcher.Close()

    for {
        event, err := watcher.Next()
        if err != nil { return }

        c.instancesMu.Lock()
        switch event.Type {
        case registry.EventAdd:
            c.instances = append(c.instances, event.Instance)
        case registry.EventDelete:
            c.instances = removeInstance(c.instances, event.Instance.ID)
            // 同时关闭该实例的连接池
            c.poolManager.RemovePool(event.Instance.Address)
        case registry.EventUpdate:
            c.instances = updateInstance(c.instances, event.Instance)
        }
        c.instancesMu.Unlock()
    }
}()
```

Watch 使实例列表保持最新，新实例注册后立即可用，实例下线后立即从候选列表移除。

## 每实例熔断器

Discovery 模式下，每个服务实例（地址）维护独立的熔断器，存储在 `sync.Map` 中：

```go
// 懒初始化：首次访问某地址时创建熔断器
func (c *Client) getBreaker(address string) *circuitbreaker.CircuitBreaker {
    v, _ := c.breakers.LoadOrStore(address,
        circuitbreaker.NewCircuitBreaker(c.opts.breakerConfig))
    return v.(*circuitbreaker.CircuitBreaker)
}
```

## 配置项完整列表

**源码**：`pkg/client/options.go`（109 行）

```go
type clientOptions struct {
    // 网络
    codec       protocol.CodecType
    compress    protocol.CompressType
    callTimeout time.Duration  // 单次 RPC 超时，默认 5s

    // 连接池
    minConnections int          // 每地址连接池下限，默认 2
    maxConnections int          // 每地址连接池上限，默认 10
    idleTimeout    time.Duration // 连接空闲超时，默认 60s

    // Discovery 模式
    discovery    registry.Discovery
    balancer     loadbalancer.LoadBalancer
    watchEnabled bool           // 是否启用后台 Watch
    cbEnabled    bool           // 是否启用熔断器
    breakerConfig circuitbreaker.Config

    // 拦截器
    interceptors []interceptor.Interceptor
}
```

| Option 函数 | 说明 | 默认值 |
|-------------|------|--------|
| `WithCodec(t)` | 序列化格式 | JSON |
| `WithCompress(t)` | 压缩算法 | None |
| `WithCallTimeout(d)` | 调用超时 | 5s |
| `WithMaxConnections(n)` | 连接池上限 | 10 |
| `WithMinConnections(n)` | 连接池下限 | 2 |
| `WithIdleTimeout(d)` | 连接空闲超时 | 60s |
| `WithDiscovery(d)` | 服务发现（Discovery 模式必须） | nil |
| `WithLoadBalancer(lb)` | 负载均衡算法 | RoundRobin |
| `WithWatch(bool)` | 启用后台实例 Watch | false |
| `WithCircuitBreaker(bool)` | 启用熔断器 | false |
| `WithClientInterceptors(...)` | 客户端拦截器 | 无 |
| `WithRateLimit(limiter)` | 启用客户端限流（自动前置到拦截器链） | 无 |
| `WithRetry(n, interval)` | 启用自动重试 | 无 |

## 关闭客户端

```go
// Close 关闭所有连接池和 Watch goroutine
defer cli.Close()
```

注意：若未调用 `Close()`，后台 Watch goroutine 和连接池中的 TCP 连接会泄漏。建议使用 `defer` 确保关闭。

## 相关文档

- [固定地址模式](fixed-mode.md) — Fixed 模式详细说明
- [服务发现模式](discovery-mode.md) — Discovery 模式详细说明
- [连接池](../transport/connection-pool.md) — 连接池实现
- [负载均衡](../loadbalancer/overview.md)
- [熔断器](../reliability/circuit-breaker.md)
