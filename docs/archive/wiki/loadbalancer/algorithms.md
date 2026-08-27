# 负载均衡算法

**源码位置**：`pkg/loadbalancer/`

## LoadBalancer 接口

```go
// pkg/loadbalancer/balancer.go
type LoadBalancer interface {
    Pick(instances []*registry.ServiceInstance) (*registry.ServiceInstance, error)
    Name() string
}

// 扩展接口：支持 Key 路由（一致性哈希）
type BalancerWithOptions interface {
    LoadBalancer
    PickWithOptions(instances []*registry.ServiceInstance,
        opts PickOptions) (*registry.ServiceInstance, error)
}

type PickOptions struct {
    Key      string            // 路由键（如用户 ID、Session ID）
    Metadata map[string]string // 额外路由元数据
}
```

---

## Round Robin（轮询）

**源码**：`pkg/loadbalancer/roundrobin.go`（33 行）

```go
type RoundRobinBalancer struct {
    counter uint64 // 原子计数器，无锁实现
}

func NewRoundRobin() *RoundRobinBalancer {
    return &RoundRobinBalancer{}
}

func (r *RoundRobinBalancer) Pick(instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
    if len(instances) == 0 {
        return nil, ErrNoAvailableInstances
    }
    idx := atomic.AddUint64(&r.counter, 1) % uint64(len(instances))
    return instances[idx], nil
}

func (r *RoundRobinBalancer) Name() string { return "round_robin" }
```

**特性**：
- 使用 `sync/atomic` 原子自增，完全无锁，极高并发性能
- 请求按索引循环分配，每个实例获得相同数量的请求
- 实例列表变化后自动适应（`instances` 由外部传入）

---

## Random（随机）

**源码**：`pkg/loadbalancer/random.go`（35 行）

```go
type RandomBalancer struct {
    mu  sync.Mutex
    rng *rand.Rand
}

func NewRandom() *RandomBalancer {
    return &RandomBalancer{
        rng: rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (r *RandomBalancer) Pick(instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
    if len(instances) == 0 {
        return nil, ErrNoAvailableInstances
    }
    r.mu.Lock()
    idx := r.rng.Intn(len(instances))
    r.mu.Unlock()
    return instances[idx], nil
}
```

**特性**：
- 使用私有 `rand.Rand`（非全局，避免竞争），加 `sync.Mutex` 保护
- 大量请求下统计上均匀，少量请求可能偏斜
- 比 Round Robin 稍慢（需要加锁），但差异可忽略

---

## Weighted Round Robin（加权轮询）

**源码**：`pkg/loadbalancer/weighted.go`（85 行）

```go
type WeightedRoundRobinBalancer struct {
    mu          sync.Mutex
    weights     []int   // 展开后的权重数组（如 weight=[3,1,2] → [0,0,0,1,2,2]）
    current     int     // 当前位置
    lastInstIDs []string // 上次构建时的实例 ID 列表（检测是否需要重建）
}

func (r *WeightedRoundRobinBalancer) Pick(
    instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {

    if len(instances) == 0 {
        return nil, ErrNoAvailableInstances
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    // 检测实例列表是否变化，若变化则重建权重数组
    if r.needsRebuild(instances) {
        r.rebuild(instances)
    }

    // 在权重数组中选择当前位置
    idx := r.weights[r.current]
    r.current = (r.current + 1) % len(r.weights)

    return instances[idx], nil
}

// 构建展开权重数组
// instances[0].Weight=3, instances[1].Weight=1, instances[2].Weight=2
// → weights = [0, 0, 0, 1, 2, 2]（实例下标重复出现 Weight 次）
func (r *WeightedRoundRobinBalancer) rebuild(instances []*registry.ServiceInstance) {
    weights := make([]int, 0)
    for i, inst := range instances {
        w := inst.Weight
        if w <= 0 {
            w = 1 // 默认权重为 1
        }
        for j := 0; j < w; j++ {
            weights = append(weights, i)
        }
    }
    r.weights = weights
    r.current = 0
    // 记录实例 ID 列表，用于下次变化检测
    r.lastInstIDs = make([]string, len(instances))
    for i, inst := range instances {
        r.lastInstIDs[i] = inst.ID
    }
}
```

**权重配置**：

通过 `ServiceInstance.Weight` 字段设置，或在注册时通过 Metadata 传递：

```go
// 服务端注册时设置权重
srv := server.NewServer(
    server.WithWeight(3), // 该实例权重为 3
)

// 或在 ServiceInstance 中直接设置
inst := &registry.ServiceInstance{
    ID:      "server-1",
    Weight:  3, // 接受 50% 流量（3/6）
}
```

---

## Consistent Hash（一致性哈希）

**源码**：`pkg/loadbalancer/consistent.go`（127 行）

### 核心数据结构

```go
type ConsistentHashBalancer struct {
    mu           sync.RWMutex
    ring         []uint32          // 排序的哈希环节点值
    nodes        map[uint32]string // hash → instanceID
    instances    map[string]*registry.ServiceInstance // instanceID → ServiceInstance
    virtualNodes int               // 每个真实节点的虚拟节点数，默认 150
}
```

### 哈希函数

使用 **MD5** 计算节点哈希：

```go
func hashKey(key string) uint32 {
    h := md5.Sum([]byte(key))
    // 取前 4 字节作为 uint32
    return uint32(h[3]) | uint32(h[2])<<8 | uint32(h[1])<<16 | uint32(h[0])<<24
}
```

### 虚拟节点

每个真实实例在哈希环上放置 `virtualNodes`（默认 150）个虚拟节点，节点键格式为 `instanceID#N`：

```go
func (r *ConsistentHashBalancer) addInstance(inst *registry.ServiceInstance) {
    for i := 0; i < r.virtualNodes; i++ {
        vKey := fmt.Sprintf("%s#%d", inst.ID, i)
        h := hashKey(vKey)
        r.ring = append(r.ring, h)
        r.nodes[h] = inst.ID
    }
    // 保持 ring 有序（用于二分查找）
    sort.Slice(r.ring, func(i, j int) bool {
        return r.ring[i] < r.ring[j]
    })
}
```

3 个实例 × 150 虚拟节点 = 450 个环节点，分布足够均匀。

### 查找（二分查找）

```go
func (r *ConsistentHashBalancer) PickWithOptions(
    instances []*registry.ServiceInstance,
    opts PickOptions) (*registry.ServiceInstance, error) {

    if len(instances) == 0 {
        return nil, ErrNoAvailableInstances
    }

    r.mu.Lock()
    r.rebuildIfNeeded(instances) // 检测实例变化，按需重建
    r.mu.Unlock()

    r.mu.RLock()
    defer r.mu.RUnlock()

    h := hashKey(opts.Key)

    // 二分查找：找到环上第一个 >= h 的位置
    idx := sort.Search(len(r.ring), func(i int) bool {
        return r.ring[i] >= h
    })

    // 超过最大值，绕回起点
    if idx == len(r.ring) {
        idx = 0
    }

    instanceID := r.nodes[r.ring[idx]]
    return r.instances[instanceID], nil
}
```

### 实例变更影响

```
3 个实例 [A, B, C]，每个 150 虚拟节点 = 450 环节点
加入新实例 D → 只有约 1/4 的 Key 重新路由到 D
删除实例 B → 只有原来路由到 B 的 Key 重新分配（约 1/3）
```

### 使用方式

```go
ch := loadbalancer.NewConsistentHash(150)

// 通过 PickWithOptions 传入路由 Key
inst, err := ch.PickWithOptions(instances, loadbalancer.PickOptions{
    Key: userID, // 同一 userID 始终路由到同一实例
})
```

当使用 `Pick()`（无 Key）时，一致性哈希退化为随机选择。

---

## 算法对比

| 特性 | Round Robin | Random | Weighted | Consistent Hash |
|------|:-----------:|:------:|:--------:|:---------------:|
| 并发安全 | 原子操作（无锁）| 互斥锁 | 互斥锁 | 读写锁 |
| 均匀性 | 精确均匀 | 统计均匀 | 按权重 | 统计均匀 |
| 粘性路由 | ❌ | ❌ | ❌ | ✅ |
| 性能感知 | ❌ | ❌ | ✅ | ❌ |
| 实例变化影响 | 无 | 无 | 重建权重数组 | ~1/N 迁移 |
| 适用场景 | **默认推荐** | 测试 | 异构集群 | 缓存/会话 |

## 相关文档

- [负载均衡概述](overview.md)
- [服务发现模式](../client/discovery-mode.md)
- [Registry 概述](../registry/overview.md) — ServiceInstance.Weight 字段
