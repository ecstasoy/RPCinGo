# 负载均衡概述

## 职责

负载均衡器在 Discovery 模式下从多个可用实例中选择一个发送请求，目标是合理分散请求，避免单节点过载。

**源码位置**：`pkg/loadbalancer/`

## 接口定义

**源码**：`pkg/loadbalancer/balancer.go`（31 行）

```go
// 基础接口
type LoadBalancer interface {
    // Pick 从实例列表中选择一个
    Pick(instances []*registry.ServiceInstance) (*registry.ServiceInstance, error)
    // Name 返回算法名称（如 "round_robin"）
    Name() string
}

// 扩展接口：支持基于 Key 的路由（一致性哈希需要）
type BalancerWithOptions interface {
    LoadBalancer
    PickWithOptions(instances []*registry.ServiceInstance,
        opts PickOptions) (*registry.ServiceInstance, error)
}

type PickOptions struct {
    Key      string            // 路由键（如用户 ID、Session ID）
    Metadata map[string]string // 额外路由元数据
}

// 所有实现都返回此错误（当 instances 为空时）
var ErrNoAvailableInstances = errors.New("no available instances")
```

## 四种实现

| 实现 | 构造函数 | 算法 | 并发安全 | 粘性 |
|------|---------|------|---------|------|
| `RoundRobinBalancer` | `NewRoundRobin()` | 顺序轮询 | 原子操作（无锁）| ❌ |
| `RandomBalancer` | `NewRandom()` | 随机 | 互斥锁 | ❌ |
| `WeightedRoundRobinBalancer` | `NewWeightedRoundRobin()` | 加权轮询 | 互斥锁 | ❌ |
| `ConsistentHashBalancer` | `NewConsistentHash(vNodes)` | 一致性哈希（MD5）| 读写锁 | ✅ |

详见 [均衡算法](algorithms.md)。

## 在 Client 中配置

```go
import "RPCinGo/pkg/loadbalancer"

// Round Robin（默认推荐）
cli, _ := client.NewDiscoveryClient(
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
)

// 随机
cli, _ := client.NewDiscoveryClient(
    client.WithLoadBalancer(loadbalancer.NewRandom()),
)

// 加权轮询（需服务端设置 instance.Weight）
cli, _ := client.NewDiscoveryClient(
    client.WithLoadBalancer(loadbalancer.NewWeightedRoundRobin()),
)

// 一致性哈希（150 虚拟节点，需通过 PickWithOptions 传入 Key）
cli, _ := client.NewDiscoveryClient(
    client.WithLoadBalancer(loadbalancer.NewConsistentHash(150)),
)
```

## 与熔断器的协作

负载均衡选出候选实例后，熔断器做最终放行/拒绝决策：

```go
// pkg/client/client.go（简化）
func (c *Client) selectInstance(instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
    // 尝试选择实例，跳过熔断中的实例
    for attempt := 0; attempt < len(instances); attempt++ {
        instance, err := c.balancer.Pick(instances)
        if err != nil {
            return nil, err
        }

        cb := c.getBreaker(instance.FullAddress())
        if cb.Allow() {
            return instance, nil // 熔断器放行
        }
        // 该实例熔断中，继续尝试其他实例
        // （Round Robin 会在下一轮选出不同的实例）
    }
    return nil, ErrAllInstancesUnavailable
}
```

当所有实例均熔断时，返回 `ErrAllInstancesUnavailable`。

## 实例列表更新时的行为

各算法在实例列表变化（Watch 事件触发）时的处理：

| 算法 | 实例增减时 |
|------|-----------|
| Round Robin | 自动适应（`idx % len(instances)`，下次 Pick 生效）|
| Random | 自动适应（重新 `rand.Intn(len(instances))`）|
| Weighted | 自动重建权重数组（`needsRebuild` 检测 ID 列表变化）|
| Consistent Hash | 自动重建哈希环（`rebuildIfNeeded` 检测变化）|

## 选择指南

```
默认/通用：        Round Robin
实例性能差异大：    Weighted Round Robin（配合 instance.Weight）
有会话/缓存亲和：  Consistent Hash（传入 sessionID 或 userID 作为 Key）
压测/随机流量：    Random
```

## 相关文档

- [均衡算法](algorithms.md) — 四种算法完整实现
- [服务发现模式](../client/discovery-mode.md) — 负载均衡在客户端中的使用
- [熔断器](../reliability/circuit-breaker.md) — 与负载均衡的协作
