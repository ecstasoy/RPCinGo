# Registry 概述

## 职责

注册中心（Registry）是微服务发现的核心基础设施，负责服务实例的注册、注销、心跳维持和实时发现。

**源码位置**：`pkg/registry/`

## 接口定义

**源码**：`pkg/registry/registry.go`（35 行）

### Registry 接口（服务端使用）

```go
type Registry interface {
    Register(ctx context.Context, instance *ServiceInstance) error
    Deregister(ctx context.Context, service, instanceID string) error
    Update(ctx context.Context, instance *ServiceInstance) error
    Heartbeat(ctx context.Context, service, instanceID string) error
    Close() error
}
```

### Discovery 接口（客户端使用）

```go
type Discovery interface {
    GetInstances(ctx context.Context, service string) ([]*ServiceInstance, error)
    Watch(ctx context.Context, service string) (Watcher, error)
    Close() error
}
```

### RegistryDiscovery（组合接口）

```go
// 同时实现注册和发现，用于内存实现（单进程测试）
type RegistryDiscovery interface {
    Registry
    Discovery
}
```

### 错误常量

```go
var (
    ErrNotFound       = errors.New("registry: service not found")
    ErrAlreadyExists  = errors.New("registry: service already exists")
    ErrNotConnected   = errors.New("registry: not connected")
    ErrWatcherStopped = errors.New("registry: watcher stopped")
)
```

## ServiceInstance 结构

**源码**：`pkg/registry/instance.go`（63 行）

```go
type ServiceInstance struct {
    ID           string            // 实例唯一 ID（通常是 "host:port" 或 UUID）
    Service      string            // 服务名称，如 "UserService"
    Version      string            // 服务版本，如 "1.0.0"（用于多版本路由）
    Address      string            // 监听地址（host），如 "10.0.0.1"
    Port         int               // 监听端口，如 8080
    Metadata     map[string]string // 扩展元数据（区域、权重、标签等）
    Weight       int               // 负载均衡权重，默认 1
    Status       InstanceStatus    // 实例状态
    RegisterTime time.Time         // 注册时间
    UpdateTime   time.Time         // 最后更新时间
}
```

### InstanceStatus 枚举

```go
type InstanceStatus int

const (
    InstanceStatusUnknown  InstanceStatus = iota // 0：未知状态
    InstanceStatusUp                             // 1：正常运行
    InstanceStatusDown                           // 2：已下线
    InstanceStatusStarting                       // 3：启动中
)
```

`GetInstances` 只返回 `InstanceStatusUp` 的实例，其他状态的实例被过滤。

### 获取完整地址

```go
func (i *ServiceInstance) FullAddress() string {
    return fmt.Sprintf("%s:%d", i.Address, i.Port)
}
```

## Watcher 接口

**源码**：`pkg/registry/watcher.go`（35 行）

```go
type Watcher interface {
    // 阻塞等待下一个事件，Watcher 关闭后返回 ErrWatcherStopped
    Next() (*Event, error)
    // 关闭 Watcher，释放底层资源
    Stop() error
}

type Event struct {
    Type     EventType
    Instance *ServiceInstance
}

type EventType int

const (
    EventAdd    EventType = iota // 实例新增（注册或从 Down→Up）
    EventUpdate                  // 实例更新（权重变更、Metadata 变更等）
    EventDelete                  // 实例删除（注销或心跳超时）
)
```

> **注意**：事件类型是 `EventAdd`/`EventUpdate`/`EventDelete`，不是 Register/Deregister。

## 服务端注册流程

```go
// 1. 创建 Registry
reg, _ := etcd.NewRegistry(
    etcd.WithEndpoints("localhost:2379"),
    etcd.WithLeaseTTL(30),
)

// 2. 在 Server 中配置 Registry
srv := server.NewServer(
    server.WithAddress("10.0.0.1:8080"),
    server.WithRegistry(reg, "UserService"),         // 服务名
    server.WithServiceVersion("1.0.0"),              // 服务版本（可选）
    server.WithHeartbeatInterval(10 * time.Second),  // 心跳间隔
)
srv.RegisterService("UserService", &UserService{})
srv.Start(ctx)
// 启动时自动执行：
//   reg.Register(ctx, &ServiceInstance{
//       Service: "UserService",
//       Address: "10.0.0.1",
//       Port: 8080,
//       Status: InstanceStatusUp,
//   })
// 并启动后台 goroutine 每 10s 调用 reg.Heartbeat(...)
// 停止时自动执行：
//   reg.Deregister(ctx, "UserService", instanceID)
```

## 客户端发现流程

```go
// 1. 创建 Discovery
disc, _ := etcd.NewDiscovery(etcd.WithEndpoints("localhost:2379"))

// 2. 按需查询
instances, _ := disc.GetInstances(ctx, "UserService")
// 只返回 Status == InstanceStatusUp 的实例

// 3. 订阅变化
watcher, _ := disc.Watch(ctx, "UserService")
for {
    event, err := watcher.Next()
    if errors.Is(err, registry.ErrWatcherStopped) {
        break // Watcher 已关闭
    }
    switch event.Type {
    case registry.EventAdd:
        fmt.Printf("新实例: %s\n", event.Instance.FullAddress())
    case registry.EventDelete:
        fmt.Printf("实例下线: %s\n", event.Instance.FullAddress())
    case registry.EventUpdate:
        fmt.Printf("实例更新: %s weight=%d\n",
            event.Instance.FullAddress(), event.Instance.Weight)
    }
}
```

## 实现对比

| 特性 | etcd 实现 | Memory 实现 |
|------|-----------|------------|
| 跨进程共享 | ✅ | ❌ |
| 持久化 | ✅（etcd 存储）| ❌ |
| TTL/心跳 | ✅（etcd 租约）| ❌（心跳为空操作）|
| 分布式一致性 | ✅（raft）| ❌ |
| 适用场景 | 生产 | 单元测试 |
| 依赖 | etcd 集群 | 无 |

## 相关文档

- [etcd 实现](etcd.md)
- [内存实现](memory.md)
- [服务发现模式](../client/discovery-mode.md)
- [Server 概述](../server/overview.md)
