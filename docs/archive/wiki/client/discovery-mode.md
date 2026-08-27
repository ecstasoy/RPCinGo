# 服务发现模式

## 概述

Discovery 模式在 Fixed 模式基础上增加了服务发现、负载均衡、熔断和实时 Watch 能力，是生产微服务场景的推荐选择。

**源码位置**：`pkg/client/client.go`

## 创建 Discovery 客户端

```go
import (
    "RPCinGo/pkg/client"
    "RPCinGo/pkg/loadbalancer"
    "RPCinGo/pkg/registry/etcd"
)

// 初始化服务发现
disc, err := etcd.NewDiscovery(
    etcd.WithEndpoints("localhost:2379"),
    etcd.WithDialTimeout(5 * time.Second),
    etcd.WithKeyPrefix("/rpc/services"),
)
if err != nil {
    log.Fatal(err)
}

// 创建客户端
cli, err := client.NewDiscoveryClient(
    client.WithDiscovery(disc),
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
    client.WithCircuitBreaker(true),
    client.WithCallTimeout(5 * time.Second),
    client.WithWatch(true),            // 启用后台实例 Watch
    client.WithMaxConnections(20),     // 每个实例最多 20 个连接
    client.WithMinConnections(2),      // 每个实例预创建 2 个连接
)
if err != nil {
    log.Fatal(err)
}
defer cli.Close() // 关闭所有连接池和 Watch goroutine
```

## 完整调用流程

```
cli.Call(ctx, "UserService", "GetUser", args)
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 1. 获取实例列表                              │
    │ │    c.instancesMu.RLock()                    │
    │ │    instances = c.instances（本地缓存）        │
    │ │    c.instancesMu.RUnlock()                  │
    │ │    若为空 → GetInstances(ctx, "UserService") │
    │ └─────────────────────────────────────────────┘
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 2. 负载均衡选择                              │
    │ │    instance = loadBalancer.Pick(instances)  │
    │ │    // 如 10.0.0.1:8081（Round Robin）        │
    │ └─────────────────────────────────────────────┘
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 3. 熔断器检查                                │
    │ │    cb = c.getBreaker("10.0.0.1:8081")       │
    │ │    if !cb.Allow() {                         │
    │ │        return nil, ErrServiceUnavailable    │
    │ │    }                                        │
    │ └─────────────────────────────────────────────┘
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 4. 从连接池取连接                            │
    │ │    pool = poolManager.GetPool("10.0.0.1:8081")│
    │ │    conn = pool.Get(ctx)                     │
    │ └─────────────────────────────────────────────┘
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 5. 发送请求 / 接收响应（同 Fixed 模式）       │
    │ └─────────────────────────────────────────────┘
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 6. 上报熔断统计                              │
    │ │    if err != nil { cb.RecordFailure() }     │
    │ │    else          { cb.RecordSuccess() }     │
    │ └─────────────────────────────────────────────┘
    │
    │ ┌─────────────────────────────────────────────┐
    │ │ 7. 归还连接                                  │
    │ │    pool.Put(conn)                           │
    │ └─────────────────────────────────────────────┘
    ▼
返回 (result, error)
```

## Watch 机制详解

`WithWatch(true)` 时，`NewDiscoveryClient` 在后台启动一个 goroutine，持续监听服务实例变化：

```go
// NewDiscoveryClient 内部（简化）
go func() {
    watcher, err := c.discovery.Watch(ctx, serviceName)
    if err != nil { return }
    defer watcher.Stop()

    for {
        event, err := watcher.Next()
        if err != nil {
            // Watcher 关闭（ErrWatcherStopped）或 ctx 超时
            return
        }

        c.instancesMu.Lock()
        switch event.Type {
        case registry.EventAdd:
            c.instances = append(c.instances, event.Instance)
            log.Printf("Instance added: %s", event.Instance.FullAddress())

        case registry.EventDelete:
            c.instances = removeInstance(c.instances, event.Instance.ID)
            // 关闭该实例的连接池（避免连接泄漏）
            c.poolManager.RemovePool(event.Instance.FullAddress())
            log.Printf("Instance removed: %s", event.Instance.FullAddress())

        case registry.EventUpdate:
            c.instances = updateInstance(c.instances, event.Instance)
            log.Printf("Instance updated: %s weight=%d",
                event.Instance.FullAddress(), event.Instance.Weight)
        }
        c.instancesMu.Unlock()
    }
}()
```

**Watch 使实例列表始终最新**：
- 新实例注册后约 <1s 就能参与负载均衡
- 实例下线（心跳超时或主动注销）后，Watch 收到 DELETE 事件，立即从候选列表移除

## 每实例独立熔断器

每个服务实例地址维护独立的熔断器，使用 `sync.Map` 懒初始化：

```go
func (c *Client) getBreaker(address string) *circuitbreaker.CircuitBreaker {
    v, _ := c.breakers.LoadOrStore(address,
        circuitbreaker.NewCircuitBreaker(c.opts.breakerConfig))
    return v.(*circuitbreaker.CircuitBreaker)
}
```

实例 A 熔断不影响实例 B，负载均衡会将流量自动引导到其他健康实例：

```
实例 10.0.0.1:8081  → Closed（正常）  ← 接收请求
实例 10.0.0.2:8081  → Open（熔断中）  ← 跳过
实例 10.0.0.3:8081  → Closed（正常）  ← 接收请求
```

## 全部实例均不可用

当所有实例都处于 Open（熔断）状态时：

```go
// 客户端内部（简化）
for _, instance := range instances {
    cb := c.getBreaker(instance.FullAddress())
    if cb.Allow() {
        return c.doCall(ctx, instance, req)
    }
}
return nil, ErrAllInstancesUnavailable
```

此时建议配合重试拦截器，等待熔断器超时后自动重试：

```go
cli, _ := client.NewDiscoveryClient(
    // ...
    client.WithInterceptors(
        interceptor.NewRetryInterceptor(2, 1*time.Second), // 等 1s 后重试
    ),
)
```

## 使用加权负载均衡

不同实例配置不同权重（如高配机器承担更多流量）：

```go
// 服务端启动时设置权重
srv := server.NewServer(
    server.WithServiceWeight(3), // 该实例权重为 3
    // ...
)

// 客户端使用 Weighted Round Robin
cli, _ := client.NewDiscoveryClient(
    client.WithLoadBalancer(loadbalancer.NewWeightedRoundRobin()),
)
```

## 使用一致性哈希

同一用户 ID 始终路由到同一实例（适合带本地缓存的服务）：

```go
cli, _ := client.NewDiscoveryClient(
    client.WithLoadBalancer(loadbalancer.NewConsistentHash(150)),
)

// 调用时传入路由 Key
cli.CallWithKey(ctx, "UserService", "GetProfile", req,
    loadbalancer.PickOptions{Key: userID})
```

## 生产推荐配置

```go
cli, err := client.NewDiscoveryClient(
    client.WithDiscovery(disc),
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
    client.WithCircuitBreaker(true),
    client.WithCircuitBreakerConfig(circuitbreaker.Config{
        MinRequests:      20,
        FailureThreshold: 0.5,
        Timeout:          10 * time.Second,
        SuccessThreshold: 2,
    }),
    client.WithCallTimeout(3 * time.Second),
    client.WithMaxConnections(50),
    client.WithMinConnections(5),
    client.WithWatch(true),
    client.WithInterceptors(
        interceptor.NewRetryInterceptor(2, 200*time.Millisecond),
        interceptor.NewLoggingInterceptor(nil),
    ),
)
```

## 相关文档

- [Client 概述](overview.md)
- [固定地址模式](fixed-mode.md)
- [etcd 注册中心](../registry/etcd.md)
- [负载均衡算法](../loadbalancer/algorithms.md)
- [熔断器](../reliability/circuit-breaker.md)
- [连接池](../transport/connection-pool.md)
