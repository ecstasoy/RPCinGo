# etcd 注册中心

## 概述

基于 etcd v3 API 实现的生产级服务注册与发现，利用 etcd 的强一致性、租约（Lease）机制和 Watch 能力保证服务实例列表的准确性和实时性。

**源码位置**：`pkg/registry/etcd/`（etcd.go 64行、registry.go 125行、discovery.go 88行、watcher.go 63行）

**依赖**：`go.etcd.io/etcd/client/v3 v3.6.7`

## 配置与初始化

```go
// pkg/registry/etcd/etcd.go
type Config struct {
    Endpoints   []string      // etcd 节点地址列表
    DialTimeout time.Duration // 连接超时，默认 5s
    KeyPrefix   string        // Key 前缀，默认 "/rpc/services"
    LeaseTTL    int64         // 租约 TTL（秒），默认 30s
}
```

### 创建 Registry（服务端）

```go
reg, err := etcd.NewRegistry(
    etcd.WithEndpoints("localhost:2379", "localhost:2380"),
    etcd.WithDialTimeout(5 * time.Second),
    etcd.WithKeyPrefix("/myapp/services"),
    etcd.WithLeaseTTL(30),
)
```

### 创建 Discovery（客户端）

```go
disc, err := etcd.NewDiscovery(
    etcd.WithEndpoints("localhost:2379"),
    etcd.WithDialTimeout(5 * time.Second),
    etcd.WithKeyPrefix("/myapp/services"),
)
```

## Key 格式

etcd 中的服务实例以如下格式存储：

```
{KeyPrefix}/{service}/{instanceID}
例：/rpc/services/UserService/10.0.0.1:8080
```

Value 是 `ServiceInstance` 的 JSON 序列化：

```json
{
  "id": "10.0.0.1:8080",
  "service": "UserService",
  "version": "1.0.0",
  "address": "10.0.0.1",
  "port": 8080,
  "metadata": {"region": "cn-north-1"},
  "weight": 1,
  "status": 1,
  "register_time": "2026-03-30T10:00:00Z",
  "update_time": "2026-03-30T10:00:00Z"
}
```

## 服务注册实现

**源码**：`pkg/registry/etcd/registry.go`（125 行）

```go
type EtcdRegistry struct {
    client  *clientv3.Client
    config  Config
    leases  sync.Map // instanceID → clientv3.LeaseID
}

func (r *EtcdRegistry) Register(ctx context.Context, inst *registry.ServiceInstance) error {
    // 1. 申请租约（TTL 秒）
    leaseResp, err := r.client.Grant(ctx, r.config.LeaseTTL)
    if err != nil {
        return fmt.Errorf("etcd grant lease: %w", err)
    }

    // 2. 序列化实例信息
    inst.Status = registry.InstanceStatusUp
    inst.RegisterTime = time.Now()
    inst.UpdateTime = time.Now()
    value, err := json.Marshal(inst)
    if err != nil {
        return err
    }

    // 3. 写入 etcd，绑定租约
    key := serviceKey(r.config.KeyPrefix, inst.Service, inst.ID)
    _, err = r.client.Put(ctx, key, string(value),
        clientv3.WithLease(leaseResp.ID))
    if err != nil {
        return fmt.Errorf("etcd put: %w", err)
    }

    // 4. 保存 LeaseID，供心跳使用
    r.leases.Store(inst.ID, leaseResp.ID)

    // 5. 启动 KeepAlive goroutine（自动维持租约）
    keepAliveCh, err := r.client.KeepAlive(ctx, leaseResp.ID)
    if err != nil {
        return err
    }
    go func() {
        for range keepAliveCh {
            // 消费 KeepAlive 响应，防止 channel 堵塞
        }
    }()

    return nil
}
```

### KeepAlive vs Heartbeat

框架支持两种心跳方式：

1. **KeepAlive（自动）**：`clientv3.KeepAlive` 内部启动 goroutine，每 TTL/3 秒自动续约，无需外部调用。注册时默认启用。

2. **Heartbeat（手动）**：`KeepAliveOnce` 单次续约，由 Server 的心跳 goroutine 按 `HeartbeatInterval` 定期调用：

```go
func (r *EtcdRegistry) Heartbeat(ctx context.Context,
    service, instanceID string) error {
    leaseID, ok := r.leases.Load(instanceID)
    if !ok {
        return registry.ErrNotFound
    }
    _, err := r.client.KeepAliveOnce(ctx, leaseID.(clientv3.LeaseID))
    return err
}
```

### 注销

```go
func (r *EtcdRegistry) Deregister(ctx context.Context,
    service, instanceID string) error {
    key := serviceKey(r.config.KeyPrefix, service, instanceID)
    _, err := r.client.Delete(ctx, key)
    if err != nil {
        return err
    }
    // 撤销租约（etcd 自动删除关联 Key）
    if leaseID, ok := r.leases.LoadAndDelete(instanceID); ok {
        r.client.Revoke(ctx, leaseID.(clientv3.LeaseID))
    }
    return nil
}
```

## 服务发现实现

**源码**：`pkg/registry/etcd/discovery.go`（88 行）

```go
type EtcdDiscovery struct {
    client *clientv3.Client
    config Config
    cache  sync.Map // service → []*ServiceInstance（本地缓存）
}

func (d *EtcdDiscovery) GetInstances(ctx context.Context,
    service string) ([]*registry.ServiceInstance, error) {

    // 1. 用前缀查询 etcd
    prefix := servicePrefix(d.config.KeyPrefix, service)
    resp, err := d.client.Get(ctx, prefix, clientv3.WithPrefix())
    if err != nil {
        return nil, fmt.Errorf("etcd get: %w", err)
    }

    // 2. 解析并过滤（只返回 Up 状态）
    instances := make([]*registry.ServiceInstance, 0, len(resp.Kvs))
    for _, kv := range resp.Kvs {
        var inst registry.ServiceInstance
        if err := json.Unmarshal(kv.Value, &inst); err != nil {
            continue // 跳过损坏的数据
        }
        if inst.Status == registry.InstanceStatusUp {
            instances = append(instances, &inst)
        }
    }

    // 3. 更新本地缓存
    d.cache.Store(service, instances)

    return instances, nil
}
```

**注意**：`GetInstances` 每次都查询 etcd，不使用缓存（缓存只在 Watch 更新时有效）。如需降低 etcd 压力，可配合 Watch 维护本地缓存，只在 Watch 失效时回退到 GetInstances。

## Watch 实现

**源码**：`pkg/registry/etcd/watcher.go`（63 行）

```go
type etcdWatcher struct {
    watchChan clientv3.WatchChan
    stopCh    chan struct{}
    closed    bool
}

func (d *EtcdDiscovery) Watch(ctx context.Context,
    service string) (registry.Watcher, error) {

    prefix := servicePrefix(d.config.KeyPrefix, service)
    watchChan := d.client.Watch(ctx, prefix,
        clientv3.WithPrefix(),
        clientv3.WithPrevKV()) // 携带前一个版本，用于 Update 检测

    return &etcdWatcher{
        watchChan: watchChan,
        stopCh:    make(chan struct{}),
    }, nil
}

func (w *etcdWatcher) Next() (*registry.Event, error) {
    select {
    case resp, ok := <-w.watchChan:
        if !ok {
            return nil, registry.ErrWatcherStopped
        }
        for _, ev := range resp.Events {
            var inst registry.ServiceInstance

            switch ev.Type {
            case mvccpb.PUT:
                json.Unmarshal(ev.Kv.Value, &inst)
                eventType := registry.EventAdd
                if ev.IsCreate() == false {
                    eventType = registry.EventUpdate // PUT 且不是首次创建 → Update
                }
                return &registry.Event{Type: eventType, Instance: &inst}, nil

            case mvccpb.DELETE:
                // DELETE 事件的 Value 为空，从 PrevKv 恢复实例信息
                if ev.PrevKv != nil {
                    json.Unmarshal(ev.PrevKv.Value, &inst)
                } else {
                    // 无前值，只能从 Key 提取 instanceID
                    inst.ID = extractInstanceID(string(ev.Kv.Key))
                }
                return &registry.Event{Type: registry.EventDelete, Instance: &inst}, nil
            }
        }
        // 空 Events（progress notify），继续等待
        return w.Next()

    case <-w.stopCh:
        return nil, registry.ErrWatcherStopped
    }
}

func (w *etcdWatcher) Stop() error {
    if !w.closed {
        w.closed = true
        close(w.stopCh)
    }
    return nil
}
```

## 容错与高可用

| 故障场景 | etcd 客户端行为 |
|---------|----------------|
| 单个 etcd 节点宕机 | 自动切换到其他节点（多节点配置）|
| etcd 网络抖动 | 客户端自动重连，Watch 自动重建 |
| 服务实例崩溃 | TTL 到期后 etcd 删除 Key，Watch 收到 DELETE 事件 |
| etcd 全集群重启 | 租约丢失，服务需重新注册（建议监控 KeepAlive channel 关闭事件）|

## 操作 etcd 验证

```bash
# 查看所有注册的服务
etcdctl get /rpc/services --prefix

# 查看特定服务的实例
etcdctl get /rpc/services/UserService --prefix

# 查看租约列表
etcdctl lease list

# 手动删除实例（模拟实例下线）
etcdctl del /rpc/services/UserService/10.0.0.1:8080
```

## 相关文档

- [Registry 概述](overview.md) — 接口定义与 ServiceInstance
- [内存实现](memory.md)
- [服务发现模式](../client/discovery-mode.md)
