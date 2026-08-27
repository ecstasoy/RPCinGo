# 内存注册中心

## 概述

内存注册中心将服务实例存储在进程内存的 map 中，同时实现 `Registry` 和 `Discovery` 接口（即 `RegistryDiscovery`），无需任何外部依赖。

**源码位置**：`pkg/registry/memory/memory.go`（176 行）

## 结构

```go
type MemoryRegistry struct {
    mu       sync.RWMutex
    services map[string]map[string]*registry.ServiceInstance
    // services[serviceName][instanceID] → ServiceInstance

    watchersMu sync.RWMutex
    watchers   map[string][]*memoryWatcher
    // watchers[serviceName] → [watcher1, watcher2, ...]
}

type memoryWatcher struct {
    service string
    events  chan *registry.Event
    stopCh  chan struct{}
    closed  bool
}
```

## 初始化

```go
import "github.com/yourname/RPCinGo/pkg/registry/memory"

// 同一个对象同时用作 Registry 和 Discovery
reg := memory.NewRegistry()
```

## 核心实现

### Register

```go
func (r *MemoryRegistry) Register(_ context.Context,
    inst *registry.ServiceInstance) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if r.services[inst.Service] == nil {
        r.services[inst.Service] = make(map[string]*registry.ServiceInstance)
    }

    inst.Status = registry.InstanceStatusUp
    inst.RegisterTime = time.Now()
    inst.UpdateTime = time.Now()
    r.services[inst.Service][inst.ID] = inst

    // 通知所有监听该服务的 Watcher
    r.notify(inst.Service, registry.EventAdd, inst)
    return nil
}
```

### GetInstances

```go
func (r *MemoryRegistry) GetInstances(_ context.Context,
    service string) ([]*registry.ServiceInstance, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    svcMap, ok := r.services[service]
    if !ok {
        return nil, registry.ErrNotFound
    }

    instances := make([]*registry.ServiceInstance, 0, len(svcMap))
    for _, inst := range svcMap {
        if inst.Status == registry.InstanceStatusUp {
            // 深拷贝，防止外部修改
            copy := *inst
            instances = append(instances, &copy)
        }
    }
    return instances, nil
}
```

### Watch

```go
func (r *MemoryRegistry) Watch(_ context.Context,
    service string) (registry.Watcher, error) {

    w := &memoryWatcher{
        service: service,
        events:  make(chan *registry.Event, 100), // 有缓冲，防止阻塞
        stopCh:  make(chan struct{}),
    }

    r.watchersMu.Lock()
    r.watchers[service] = append(r.watchers[service], w)
    r.watchersMu.Unlock()

    return w, nil
}

func (w *memoryWatcher) Next() (*registry.Event, error) {
    select {
    case event := <-w.events:
        return event, nil
    case <-w.stopCh:
        return nil, registry.ErrWatcherStopped
    }
}

func (w *memoryWatcher) Stop() error {
    if !w.closed {
        w.closed = true
        close(w.stopCh)
    }
    return nil
}
```

### notify（事件推送）

```go
func (r *MemoryRegistry) notify(service string,
    eventType registry.EventType,
    inst *registry.ServiceInstance) {

    r.watchersMu.RLock()
    watchers := r.watchers[service]
    r.watchersMu.RUnlock()

    event := &registry.Event{Type: eventType, Instance: inst}
    for _, w := range watchers {
        select {
        case w.events <- event:
        default:
            // Watcher 缓冲区满，丢弃事件（日志警告）
        }
    }
}
```

### Heartbeat（空操作）

```go
func (r *MemoryRegistry) Heartbeat(_ context.Context,
    service, instanceID string) error {
    // 内存实现无 TTL 机制，心跳为空操作
    return nil
}
```

## 在测试中使用

```go
func TestCalculatorService(t *testing.T) {
    ctx := context.Background()
    reg := memory.NewRegistry()

    // 启动服务端
    srv := server.NewServer(
        server.WithAddress("127.0.0.1:0"), // 随机端口，避免端口冲突
        server.WithRegistry(reg, "Calculator"),
    )
    srv.RegisterService("Calculator", &CalculatorService{})
    go srv.Start(ctx)
    time.Sleep(50 * time.Millisecond) // 等待启动

    // 创建客户端（同一进程，共享 reg）
    cli, _ := client.NewDiscoveryClient(
        client.WithDiscovery(reg),
        client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
    )
    defer cli.Close()

    req := &calculator.AddRequest{A: 10, B: 20}
    resp := &calculator.AddResponse{}
    err := cli.CallTyped(ctx, "Calculator", "Add", req, resp)

    assert.NoError(t, err)
    assert.Equal(t, int64(30), resp.Result)
}
```

## 局限性

| 特性 | 说明 |
|------|------|
| 跨进程共享 | ❌ 仅限同一进程内 |
| 持久化 | ❌ 进程重启后丢失 |
| TTL/心跳 | ❌ `Heartbeat` 为空操作 |
| 并发安全 | ✅ 使用 `sync.RWMutex` |
| Watch 事件 | ✅ 支持（有缓冲 channel）|

## 相关文档

- [Registry 概述](overview.md) — RegistryDiscovery 接口
- [etcd 实现](etcd.md) — 生产环境使用
