# 集成层文档 (Integration Layer)

## 📋 目录

- [概述](#概述)
- [核心组件](#核心组件)
- [设计原理](#设计原理)
- [使用指南](#使用指南)
- [完整示例](#完整示例)
- [最佳实践](#最佳实践)

---

## 概述

### 什么是集成层？

集成层将 Registry（服务注册发现）整合到 RPC Client/Server 中，实现真正的**分布式 RPC 调用**。

### 核心价值

```
Before Integration (固定地址):
  Client → "localhost:8080" (硬编码)
  
  问题：
  ❌ 无法动态扩容
  ❌ 实例故障无法切换
  ❌ 无法负载均衡

After Integration (服务发现):
  Client → Registry → [Server1, Server2, Server3]
  
  优势：
  ✅ 自动发现服务
  ✅ 动态扩缩容
  ✅ 故障自动切换
  ✅ 负载均衡
```

---

## 核心组件

### 1. PoolManager（连接池管理器）

**文件**：`pkg/client/pool_manager.go`

**作用**：管理多个地址的连接池

#### 为什么需要？

```
场景：3 个服务实例
  - 192.168.1.10:8080
  - 192.168.1.11:8080
  - 192.168.1.12:8080

需求：
  每个地址一个连接池
  复用连接，提升性能

PoolManager:
  pools["192.168.1.10:8080"] → Pool1 (10 connections)
  pools["192.168.1.11:8080"] → Pool2 (10 connections)
  pools["192.168.1.12:8080"] → Pool3 (10 connections)
```

#### 核心方法

```go
type PoolManager struct {
    pools map[string]*tcp.ConnectionPool
}

// 获取连接（按需创建池）
func (pm *PoolManager) GetConnection(ctx, address) (*PooledConnection, error)

// 移除池（实例下线）
func (pm *PoolManager) RemovePool(address) error

// 关闭所有池
func (pm *PoolManager) Close() error

// 统计信息
func (pm *PoolManager) Stats() map[string]PoolStats
```

#### 设计特点

**双重检查锁（Double-Checked Locking）**：

```go
// 第一次检查（读锁）
pm.mu.RLock()
pool, exists := pm.pools[address]
pm.mu.RUnlock()

if exists {
    return pool.Get()  // 快速返回
}

// 第二次检查（写锁）
pm.mu.Lock()
pool, exists = pm.pools[address]  // 再次检查！
if !exists {
    pool = createPool(address)  // 创建
    pm.pools[address] = pool
}
pm.mu.Unlock()

return pool.Get()
```

**为什么第二次检查？**

避免并发时重复创建：
```
时间线：
G1: 第一次检查（无）→ 准备创建
G2: 第一次检查（无）→ 准备创建

没有第二次检查:
G1: Lock → 创建 pool1 → 存储
G2: Lock → 创建 pool2 → 覆盖！← pool1 泄漏

有第二次检查:
G1: Lock → 检查无 → 创建 pool1 → 存储
G2: Lock → 检查有！→ 使用 pool1 ← 正确
```

---

### 2. RPC Server 自动注册

**文件**：`pkg/server/server.go`

**功能**：Server 启动时自动注册到 Registry

#### 工作流程

```
1. Server 启动
   server.Start(ctx)
   ↓
2. 监听端口
   transport.Listen(":8080")
   ↓
3. 注册服务
   instance := ServiceInstance{
       Service: "UserService",
       Address: "192.168.1.10",
       Port:    8080,
   }
   registry.Register(instance)
   ↓
4. 启动心跳
   go heartbeat()  // 每 5 秒
   ↓
5. 开始服务
   transport.Serve()
```

#### 心跳机制

```go
func (s *Server) startHeartbeat() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            s.registry.Heartbeat(ctx, service, instanceID)
        case <-s.stopHeartbeat:
            return
        }
    }
}
```

**作用**：
- Memory Registry: 更新 UpdateTime
- etcd Registry: 续约 Lease（保持注册）

#### 优雅关闭

```go
func (s *Server) Stop() error {
    // 1. 停止心跳
    close(s.stopHeartbeat)
    
    // 2. 注销服务
    s.registry.Deregister(ctx, service, instanceID)
    // → 客户端立即收到通知（Watch）
    
    // 3. 关闭网络
    s.transport.Close()
    // → 等待现有连接完成
    
    return nil
}
```

**关键**：先注销再关闭，避免客户端发送请求到已关闭的服务

---

### 3. RPC Client 自动发现

**文件**：`pkg/client/client.go`

**功能**：Client 自动发现和调用服务

#### 两种模式

**Fixed Mode（固定地址）**：

```go
client := NewClient("localhost:8080")
// 简单、直接，适合测试
```

**Discovery Mode（服务发现）**：

```go
client := NewClientWithDiscovery(
    WithDiscovery(etcdDiscovery),
)
// 自动发现、负载均衡、故障切换
```

#### 调用流程

```go
client.Call("UserService", "GetUser", args)
    ↓
1. getInstances("UserService")
   ├─ 检查缓存
   │  有 → 返回缓存
   │  无 → 查询 Discovery
   ↓
2. discovery.GetInstances("UserService")
   → [instance1, instance2, instance3]
   ↓
3. 更新缓存
   instanceCache["UserService"] = instances
   ↓
4. 启动 Watch（首次）
   go watchService("UserService")
   ↓
5. loadBalancer.Pick(instances)
   → instance2 (Round Robin)
   ↓
6. poolManager.GetConnection(instance2.Endpoint())
   → conn from pool2
   ↓
7. conn.SendRequest(request)
   → response
```

#### 缓存机制

```go
type Client struct {
    instanceCache map[service][]*ServiceInstance
    cacheMu       sync.RWMutex
}

// 为什么缓存？
Without cache:
  每次调用都查 Registry → 1-3ms 延迟

With cache:
  第一次查 Registry（1-3ms）
  后续从内存读（< 1μs）
  Watch 更新缓存（实时）
  
性能提升: 1000-3000 倍！
```

#### Watch 机制

```go
func (c *Client) watchService(service string) {
    watcher, _ := c.discovery.Watch(ctx, service)
    
    for {
        event, _ := watcher.Next()  // 阻塞等待事件
        
        switch event.Type {
        case EventTypeAdd:
            // 新实例上线
            c.cacheMu.Lock()
            c.instanceCache[service] = append(..., event.Instance)
            c.cacheMu.Unlock()
            
        case EventTypeDelete:
            // 实例下线
            c.cacheMu.Lock()
            c.instanceCache[service] = remove(event.Instance)
            c.cacheMu.Unlock()
            
            // 关闭该地址的连接池
            c.poolManager.RemovePool(event.Instance.Endpoint())
            
        case EventTypeUpdate:
            // 实例更新
            c.cacheMu.Lock()
            c.instanceCache[service] = update(event.Instance)
            c.cacheMu.Unlock()
        }
    }
}
```

**实时性**：
- etcd Watch: < 10ms 通知
- 更新缓存: < 1μs
- 关闭连接池: < 1ms

**总延迟**：< 11ms（实例下线到客户端感知）

---

### 4. 简单负载均衡器

**文件**：`pkg/client/balancer.go`

**实现**：Round Robin（轮询）

```go
type RoundRobinBalancer struct {
    index uint64  // 使用 atomic 操作
}

func (rb *RoundRobinBalancer) Pick(instances []*ServiceInstance) (*ServiceInstance, error) {
    if len(instances) == 0 {
        return nil, ErrNoInstances
    }
    
    idx := atomic.AddUint64(&rb.index, 1) % uint64(len(instances))
    return instances[idx], nil
}
```

**工作原理**：

```
实例: [A, B, C]

调用 1: index=0 % 3 = 0 → A
调用 2: index=1 % 3 = 1 → B
调用 3: index=2 % 3 = 2 → C
调用 4: index=3 % 3 = 0 → A  (循环)

特点:
✅ 均匀分布
✅ 简单高效
✅ 并发安全（atomic）
```

---

## 设计原理

### 1. 分层职责

```
Application:
  "调用 UserService"
    ↓
RPC Client (集成层):
  - 服务发现（Registry）
  - 负载均衡（Balancer）
  - 连接管理（PoolManager）
    ↓
Transport:
  - 网络传输
    ↓
Codec:
  - 序列化
```

**每层职责单一，组合协作**

---

### 2. 自动化设计

```
Server:
  Start() → 自动注册
  Stop()  → 自动注销
  
  开发者不需要手动管理！

Client:
  Call() → 自动发现 + 负载均衡 + 连接复用
  
  开发者只需要调用，其他都自动化！
```

---

### 3. 容错设计

```
场景：实例故障

1. Server crash
   ↓
2. 停止心跳（自动）
   ↓
3. Lease 过期（10 秒）
   ↓
4. etcd 删除注册（自动）
   ↓
5. Watch 通知 Client
   ↓
6. Client 更新缓存（删除故障实例）
   ↓
7. 下次调用自动路由到健康实例

Total: < 11 秒故障感知和恢复
```

---

## 使用指南

### Server 端使用

```go
package main

import (
    "context"
    "github.com/ecstasoy/RPCinGo/pkg/server"
    "github.com/ecstasoy/RPCinGo/pkg/registry/etcd"
)

func main() {
    // 1. 创建 Registry
    config := etcd.DefaultConfig()
    config.Endpoints = []string{"localhost:2379"}
    
    reg, err := etcd.NewEtcdRegistry(config)
    if err != nil {
        panic(err)
    }
    defer reg.Close()
    
    // 2. 创建 Server（配置自动注册）
    srv := server.NewServer(
        server.WithAddress(":8080"),
        server.WithRegistry("UserService", "v1.0.0", reg),
    )
    
    // 3. 注册方法
    srv.RegisterMethod("UserService", "GetUser", func(args interface{}) (interface{}, error) {
        m := args.(map[string]interface{})
        userID := int(m["id"].(float64))
        
        return map[string]interface{}{
            "id":   userID,
            "name": "User" + fmt.Sprint(userID),
        }, nil
    })
    
    // 4. 启动（自动注册到 etcd）
    ctx := context.Background()
    if err := srv.Start(ctx); err != nil {
        panic(err)
    }
    
    // 5. 关闭时自动注销
    defer srv.Stop()
}
```

**自动化流程**：
1. `Start()` → 自动注册到 etcd
2. 心跳自动启动（每 5 秒）
3. `Stop()` → 自动注销

---

### Client 端使用

#### 方式 1：固定地址（简单）

```go
import "github.com/ecstasoy/RPCinGo/pkg/client"

// 单实例场景
client, _ := client.NewClient("localhost:8080")
defer client.Close()

result, _ := client.Call(context.Background(), 
    "UserService", "GetUser", 
    map[string]interface{}{"id": 123})
```

#### 方式 2：服务发现（推荐）

```go
import (
    "github.com/ecstasoy/RPCinGo/pkg/client"
    "github.com/ecstasoy/RPCinGo/pkg/registry/etcd"
)

// 1. 创建 Discovery
config := etcd.DefaultConfig()
disc, _ := etcd.NewEtcdDiscovery(config)
defer disc.Close()

// 2. 创建 Client（配置服务发现）
cli, _ := client.NewClientWithDiscovery(
    client.WithDiscovery(disc),
)
defer cli.Close()

// 3. 调用（自动发现 + 负载均衡）
result, _ := cli.Call(context.Background(),
    "UserService", "GetUser",
    map[string]interface{}{"id": 123})

// 内部流程：
// 1. 查询 etcd → 获取所有 UserService 实例
// 2. 负载均衡 → 选择一个实例
// 3. 获取连接 → 从该实例的连接池
// 4. 发起调用
```

---

## 完整示例

### 端到端示例（多实例）

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/ecstasoy/RPCinGo/pkg/client"
    "github.com/ecstasoy/RPCinGo/pkg/server"
    "github.com/ecstasoy/RPCinGo/pkg/registry/memory"
)

func main() {
    // 共享的 Registry
    reg := memory.NewRegistry()
    defer reg.Close()
    
    // === 启动 3 个 Server ===
    for i := 0; i < 3; i++ {
        srv := server.NewServer(
            server.WithAddress(fmt.Sprintf(":808%d", i)),
            server.WithRegistry("Calculator", "v1.0.0", reg),
        )
        
        serverID := i
        srv.RegisterMethod("Calculator", "GetID", func(args interface{}) (interface{}, error) {
            return serverID, nil
        })
        
        go srv.Start(context.Background())
    }
    
    time.Sleep(500 * time.Millisecond)
    
    // === 创建 Client ===
    cli, _ := client.NewClientWithDiscovery(
        client.WithDiscovery(reg),
    )
    defer cli.Close()
    
    // === 发起多次调用（观察负载均衡）===
    for i := 0; i < 9; i++ {
        result, _ := cli.Call(context.Background(), 
            "Calculator", "GetID", nil)
        
        serverID := int(result.(float64))
        fmt.Printf("Call %d → Server %d\n", i, serverID)
    }
    
    // 输出（Round Robin）:
    // Call 0 → Server 0
    // Call 1 → Server 1
    // Call 2 → Server 2
    // Call 3 → Server 0  (循环)
    // Call 4 → Server 1
    // ...
}
```

---

## 设计模式应用

### 1. 观察者模式（Observer）

```
Subject: Registry (etcd)
Observers: Clients (via Watch)

当 Registry 变化:
  Registry → 通知所有 Watchers
  Watchers → 更新本地缓存
```

### 2. 策略模式（Strategy）

```
LoadBalancer 接口 (策略接口):
  - RoundRobinBalancer
  - RandomBalancer
  - ConsistentHashBalancer
  
Client 使用策略:
  client.loadBalancer.Pick(instances)
  // 可以动态替换策略
```

### 3. 工厂模式（Factory）

```
PoolManager 工厂:
  根据地址创建连接池
  
  pm.GetConnection(addr)
    → 检查是否存在
    → 不存在则创建
    → 返回连接
```

### 4. 缓存模式（Cache）

```
Client 缓存:
  instanceCache[service] = instances
  
  优点：
  ✅ 减少 Registry 查询
  ✅ 降低延迟
  ✅ 提升性能
  
  更新：
  ✅ Watch 实时更新
```

---

## 性能特点

### PoolManager

```
首次连接:  创建新池（5-10ms）
后续连接:  复用池（< 1μs）

多地址管理:
  3 个实例 × 10 连接/池 = 30 个连接
  内存占用: ~1MB
  
查找性能:
  map 查找: O(1) - 双重检查锁优化
```

### 缓存 + Watch

```
服务发现延迟:
  首次:    1-3ms (查询 etcd)
  后续:    < 1μs (内存缓存)
  
实例变化感知:
  etcd Watch: < 10ms
  缓存更新:   < 1μs
  
总延迟: < 11ms (实时)
```

### 端到端性能

```
完整 RPC 调用（多实例）:
  服务发现:   < 1μs (缓存)
  负载均衡:   < 1μs (Round Robin)
  获取连接:   < 1μs (池复用)
  RPC 调用:   100μs (网络 + 处理)
  ──────────────────────────
  总延迟:    ~102μs

vs 固定地址:
  RPC 调用:   100μs
  
开销: 仅 2μs（可忽略）
```

---

## 最佳实践

### 1. 使用 Watch

```go
// ✅ 推荐（启用 Watch）
client := NewClientWithDiscovery(
    WithDiscovery(disc),
    WithWatch(true),  // 默认启用
)

// ❌ 不推荐（禁用 Watch）
client := NewClientWithDiscovery(
    WithDiscovery(disc),
    WithWatch(false),  // 只在首次查询
)
// 问题：实例变化无法感知
```

### 2. 合理的缓存策略

```go
// 缓存 + Watch 结合
getInstances():
  1. 检查缓存（快速）
  2. 无缓存 → 查询 Discovery
  3. 更新缓存
  4. 启动 Watch（后续实时更新）

优点：
  ✅ 首次查询后缓存
  ✅ 实时更新缓存
  ✅ 无需定期轮询
```

### 3. 资源管理

```go
// ✅ 正确
defer client.Close()
// 关闭：
// - 所有 watchers
// - poolManager (所有连接池)

// ❌ 错误  
// 不关闭 → 连接泄漏、goroutine 泄漏
```

### 4. 错误处理

```go
result, err := client.Call(ctx, service, method, args)
if err != nil {
    // 可能的错误：
    // - 无可用实例
    // - 连接失败
    // - 调用超时
    // - 远程错误
    
    // 根据错误类型决定是否重试
    if isRetryable(err) {
        // retry
    }
}
```

---

## 测试

### E2E 测试场景

```
测试 1: 单实例服务发现
  1. 启动 1 个 Server（自动注册）
  2. Client 调用（自动发现）
  3. 验证成功
  4. Server 关闭（自动注销）
  5. 验证 Client 无法调用

测试 2: 多实例负载均衡
  1. 启动 3 个 Server
  2. Client 发起 9 次调用
  3. 验证分布均匀（每个 Server 3 次）
  
测试 3: 实例动态上下线
  1. 启动 2 个 Server
  2. Client 开始调用
  3. 关闭 Server1
  4. 验证 Client 自动切换到 Server2
```

---

## 故障场景处理

### 场景 1：实例故障

```
时间线：
t=0s:  3 个实例运行
t=5s:  Server1 crash（无法心跳）
t=15s: Lease 过期 → etcd 删除
t=15s: Watch 通知 Client
t=15s: Client 删除缓存和连接池
t=16s: 下次调用只路由到 Server2/3

影响：
  - 正在进行的连接：可能失败（需要重试）
  - 新的调用：自动避开故障实例
  
恢复时间：10-15 秒
```

### 场景 2：网络分区

```
场景：Client 无法连接 Registry

措施：
  1. 使用本地缓存（继续服务）
  2. 定期重试连接 Registry
  3. 日志告警
  
降级：
  - 无法感知新实例上线
  - 可以继续使用已知实例
```

### 场景 3：所有实例下线

```
场景：服务完全不可用

Client 行为：
  getInstances() → []  (空列表)
  Pick() → error: "no available instances"
  Call() → 返回错误
  
建议：
  - 实现重试逻辑
  - 降级处理（返回默认值/缓存）
```

---

## 对比

### Before Integration

```
Server:
  server := NewServer(...)
  server.Start()
  // 固定地址运行

Client:
  client := NewClient("localhost:8080")
  client.Call(...)
  // 硬编码地址

限制：
  ❌ 单实例（无扩展）
  ❌ 故障无法切换
  ❌ 手动管理地址
```

### After Integration

```
Server:
  server := NewServer(
      WithRegistry("Service", "v1.0", registry),
  )
  server.Start()  // 自动注册
  
Client:
  client := NewClientWithDiscovery(
      WithDiscovery(discovery),
  )
  client.Call(...)  // 自动发现 + 负载均衡
  
能力：
  ✅ 多实例（水平扩展）
  ✅ 自动故障切换
  ✅ 负载均衡
  ✅ 零配置（自动化）
```

---

## 下一步扩展

### 可以添加的功能

```
🔜 更多负载均衡算法
   - Weighted Round Robin
   - Consistent Hash
   - Least Connection
   - P2C

🔜 健康检查
   - 主动探测实例健康
   - 自动标记 DOWN

🔜 服务路由
   - 按版本路由
   - 按区域路由
   - 灰度发布

🔜 故障重试
   - 自动重试
   - 退避策略
   - 熔断保护
```

---

## 总结

### Integration 层的价值

```
技术价值：
✅ 真正的分布式 RPC
✅ 服务治理能力
✅ 生产级可用

业务价值：
✅ 支持水平扩展
✅ 高可用性
✅ 降低运维成本

学习价值：
✅ 分布式系统核心概念
✅ 服务注册发现机制
✅ 负载均衡原理
```

### 完成的功能

```
✅ PoolManager (多地址连接池管理)
✅ Server auto-registration (自动注册)
✅ Client auto-discovery (自动发现)
✅ Simple load balancing (Round Robin)
✅ Real-time updates (Watch)
✅ E2E tests (端到端测试)
```

---

**文档版本**: v1.0  
**最后更新**: 2026-01-03  
**作者**: Kunhua Huang




