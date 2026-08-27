# 负载均衡器文档 (Load Balancer)

## 概述

### 作用

负载均衡器负责从多个服务实例中**选择一个**进行调用，实现流量分配。

### 核心价值

```
Without LoadBalancer:
  总是调用第一个实例 → 负载不均

With LoadBalancer:
  智能分配 → 均匀负载 → 高性能
```

---

## 支持的算法

### 1. Round Robin (轮询)

**原理**: 依次轮流选择

```
实例: [A, B, C]
调用: A → B → C → A → B → C ...

特点:
✅ 简单
✅ 均匀分布
✅ 无状态

适用: 实例性能相同的场景
性能: 8 ns/op
```

---

### 2. Weighted Round Robin (加权轮询)

**原理**: 按权重分配

```
实例:
  A (weight=50)
  B (weight=30)
  C (weight=20)

分配比例:
  A: 50%
  B: 30%
  C: 20%

特点:
✅ 考虑实例性能差异
✅ 灵活配置

适用: 异构服务器
性能: 17 ns/op
```

---

### 3. Random (随机)

**原理**: 随机选择

```
实例: [A, B, C]
调用: B → A → C → A → B ...

特点:
✅ 实现简单
✅ 无状态
✅ 高并发友好

适用: 高并发场景
性能: 5.7 ns/op (最快!)
```

---

### 4. Consistent Hash (一致性哈希)

**原理**: 哈希环路由

```
场景: 缓存服务

User123 → hash(123) → Instance A
User456 → hash(456) → Instance B

特点:
✅ 同一 key 总是路由到同一实例
✅ 实例变化影响小
✅ 适合缓存

适用:
  - 缓存服务
  - 会话保持
  - 有状态服务

性能: 130 μs/op (需要计算)

Virtual Nodes:
  每个实例 150 个虚拟节点
  → 分布更均匀
```

---

## 算法对比

| 算法 | 性能 | 分布 | 状态 | 适用场景 |
|------|------|------|------|---------|
| Random | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 无 | 高并发 |
| RoundRobin | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 有 | 通用 |
| Weighted RR | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 有 | 异构服务器 |
| Consistent Hash | ⭐⭐⭐ | ⭐⭐⭐⭐ | 有 | 缓存/会话 |

---

## 使用示例

### 基础使用

```go
import "RPCinGo/pkg/loadbalancer"

instances := getInstances()

// Round Robin
lb := loadbalancer.NewRoundRobin()
inst, _ := lb.Pick(ctx, instances)

// Weighted
lb := loadbalancer.NewWeightedRoundRobin()
inst, _ := lb.Pick(ctx, instances)

// Random
lb := loadbalancer.NewRandom()
inst, _ := lb.Pick(ctx, instances)

// Consistent Hash (需要 key)
lb := loadbalancer.NewConsistentHash()
inst, _ := lb.(loadbalancer.BalancerWithOptions).PickWithOptions(
    ctx, instances, 
    &loadbalancer.PickOptions{Key: "user123"},
)
```

---

### 集成到 RPC Client

```go
// 默认 (Round Robin)
client := client.NewClientWithDiscovery(
    client.WithDiscovery(discovery),
)

// 使用加权轮询
client := client.NewClientWithDiscovery(
    client.WithDiscovery(discovery),
    client.WithLoadBalancer(loadbalancer.NewWeightedRoundRobin()),
)

// 使用一致性哈希
client := client.NewClientWithDiscovery(
    client.WithDiscovery(discovery),
    client.WithLoadBalancer(loadbalancer.NewConsistentHash()),
)
```

---

## 性能测试结果

```
Benchmark Results (Apple M1 Pro):

BenchmarkBalancers/Random          200M ops    5.7 ns/op   0 allocs
BenchmarkBalancers/RoundRobin      151M ops    8.1 ns/op   0 allocs
BenchmarkBalancers/Weighted         74M ops   17.4 ns/op   0 allocs
BenchmarkBalancers/ConsistentHash   10K ops  130 μs/op   919 allocs

结论:
  - Random 最快 (但分布略不均)
  - RoundRobin 次快 (分布完美)
  - Weighted 可接受 (功能强大)
  - ConsistentHash 较慢 (特殊场景必需)
```

---

## 设计原理

### 1. 接口抽象

```go
type Balancer interface {
    Pick(instances) (*Instance, error)
    Name() string
}

// 扩展接口
type BalancerWithOptions interface {
    Balancer
    PickWithOptions(instances, opts) (*Instance, error)
}

好处:
✅ 统一接口
✅ 可替换
✅ 易扩展
```

### 2. 并发安全

```
RoundRobin: atomic 操作
  atomic.AddUint64(&index, 1)

Weighted: mutex 保护
  mu.Lock() → rebuild → mu.Unlock()

ConsistentHash: RWMutex
  rebuild: Lock
  pick: RLock
```

---

## 最佳实践

### 1. 选择合适的算法

```
通用场景 → RoundRobin
  - 实例性能相同
  - 无状态服务

异构服务器 → WeightedRoundRobin
  - 高性能机器 weight=200
  - 低性能机器 weight=100

缓存服务 → ConsistentHash
  - 需要会话保持
  - 有状态服务

高并发 → Random
  - 追求极致性能
  - 可接受轻微不均
```

### 2. 权重设置

```go
// 合理的权重范围: 1-1000

instance.Weight = 100   // 标准
instance.Weight = 200   // 2倍性能
instance.Weight = 50    // 半性能
instance.Weight = 1     // 最小（几乎不用）
```

### 3. Consistent Hash 使用

```go
// ✅ 正确: 使用稳定的 key
opts := &PickOptions{
    Key: fmt.Sprintf("user-%d", userID),
}

// ❌ 错误: 使用随机 key
opts := &PickOptions{
    Key: time.Now().String(),  // 每次都不同！
}
```

---

## 测试覆盖

```
Test Cases: 4
Coverage:   76.9%
Performance: 4 benchmarks

验证:
✅ 分布均匀性
✅ 权重准确性
✅ 一致性保持
✅ 边界情况
```

---

**文档版本**: v1.0  
**最后更新**: 2026-01-03  
**作者**: Kunhua Huang




