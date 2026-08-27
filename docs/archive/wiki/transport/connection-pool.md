# 连接池

## 概述

连接池避免了为每次 RPC 调用建立新 TCP 连接的开销（TCP 握手约 1–3ms），通过复用已建立的连接将调用延迟从毫秒级降至微秒级。

**源码位置**：`pkg/pool/pool.go`（832 行）、`pkg/pool/pool_manager.go`（112 行）

## PooledConnection（池中的连接包装器）

每个从连接池取出的连接被 `PooledConnection` 包装，记录生命周期信息：

```go
type PooledConnection struct {
    *tcp.Client           // 底层 TCP 连接
    pool      *ConnectionPool
    createdAt time.Time   // 连接创建时间（用于 MaxLifetime 检查）
    lastUsed  time.Time   // 最后使用时间（用于 IdleTimeout 检查）
    inUse     bool        // 当前是否被借出
}
```

连接归还时（`pool.Put()`），框架会检查 `lastUsed` 和 `createdAt` 决定是否丢弃。

## ConnectionPool

### 核心字段

```go
type ConnectionPool struct {
    address string
    opts    PoolOptions

    conns    chan *PooledConnection  // 空闲连接队列（buffered channel）
    factory  ConnectionFactory       // 创建新连接的工厂
    validator PoolValidator          // 验证连接是否存活

    mu       sync.Mutex
    size     int          // 当前总连接数（含借出中的）
    closed   bool

    // 统计（原子操作）
    stats PoolStats
}
```

### PoolOptions

```go
type PoolOptions struct {
    MinSize             int           // 预创建最小连接数，默认 2
    MaxSize             int           // 最大连接数，默认 10
    IdleTimeout         time.Duration // 空闲超时，默认 60s（超时则关闭）
    MaxLifetime         time.Duration // 最长存活，默认 30min（到期则关闭）
    HealthCheckInterval time.Duration // 后台健康检查间隔，默认 30s
    DialTimeout         time.Duration // 建立新连接超时，默认 5s
    ReadTimeout         time.Duration // 读超时，默认 30s
    WriteTimeout        time.Duration // 写超时，默认 30s
    Logger              logger.Logger // 日志实现，默认 logger.Nop()（静默）
}
```

`Logger` 字段通过 `WithPoolLogger(l)` Option 注入，不传则静默运行：

```go
pool, _ := pool.NewConnectionPool("127.0.0.1:8080",
    pool.WithPoolLogger(logger.New()), // 启用连接池日志输出
)
```

### ConnectionFactory 接口

连接创建逻辑通过工厂接口注入，实现连接池与传输实现解耦：

```go
type ConnectionFactory interface {
    Create(address string) (*tcp.Client, error)
}
```

框架提供两种内置工厂：

```go
// DefaultConnectionFactory：直接建立 TCP 连接
type DefaultConnectionFactory struct {
    opts ClientOptions
}
func (f *DefaultConnectionFactory) Create(addr string) (*tcp.Client, error) {
    c := tcp.NewClient(f.opts)
    return c, c.Dial(addr)
}

// RetryConnectionFactory：失败时自动重试（指数退避）
type RetryConnectionFactory struct {
    inner      ConnectionFactory
    maxRetries int
    baseDelay  time.Duration
}
func (f *RetryConnectionFactory) Create(addr string) (*tcp.Client, error) {
    delay := f.baseDelay
    for i := 0; i <= f.maxRetries; i++ {
        conn, err := f.inner.Create(addr)
        if err == nil {
            return conn, nil
        }
        time.Sleep(delay)
        delay *= 2 // 指数退避
    }
    return nil, ErrMaxRetriesExceeded
}
```

### PoolValidator

```go
type PoolValidator interface {
    IsValid(conn *PooledConnection) bool
}

// 默认实现：检查连接是否存活 + 未超过生命周期
type DefaultPoolValidator struct {
    idleTimeout time.Duration
    maxLifetime time.Duration
}

func (v *DefaultPoolValidator) IsValid(conn *PooledConnection) bool {
    now := time.Now()
    // 检查空闲超时
    if v.idleTimeout > 0 && now.Sub(conn.lastUsed) > v.idleTimeout {
        return false
    }
    // 检查最大生命周期
    if v.maxLifetime > 0 && now.Sub(conn.createdAt) > v.maxLifetime {
        return false
    }
    // 检查 TCP 连接是否存活
    return conn.IsConnected()
}
```

### 获取连接（Get）

```go
func (p *ConnectionPool) Get(ctx context.Context) (*PooledConnection, error) {
    for {
        select {
        case conn := <-p.conns: // 尝试从空闲队列取
            if p.validator.IsValid(conn) {
                conn.lastUsed = time.Now()
                conn.inUse = true
                atomic.AddInt64(&p.stats.GetCount, 1)
                return conn, nil
            }
            // 连接无效，丢弃并减少计数
            conn.Close()
            p.mu.Lock()
            p.size--
            p.mu.Unlock()

        default:
            p.mu.Lock()
            if p.size < p.opts.MaxSize {
                // 池未满，创建新连接
                p.size++
                p.mu.Unlock()
                conn, err := p.createConn()
                if err != nil {
                    p.mu.Lock()
                    p.size--
                    p.mu.Unlock()
                    return nil, err
                }
                atomic.AddInt64(&p.stats.CreateCount, 1)
                return conn, nil
            }
            p.mu.Unlock()

            // 池已满，等待有连接归还或 ctx 超时
            select {
            case conn := <-p.conns:
                if p.validator.IsValid(conn) {
                    conn.lastUsed = time.Now()
                    conn.inUse = true
                    return conn, nil
                }
                // 连接无效，继续外层循环
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }
    }
}
```

### 归还连接（Put）

```go
func (p *ConnectionPool) Put(conn *PooledConnection) error {
    if p.closed {
        conn.Close()
        p.mu.Lock()
        p.size--
        p.mu.Unlock()
        return ErrPoolClosed
    }

    conn.inUse = false
    conn.lastUsed = time.Now()

    if !p.validator.IsValid(conn) {
        conn.Close()
        p.mu.Lock()
        p.size--
        p.mu.Unlock()
        atomic.AddInt64(&p.stats.CloseCount, 1)
        return nil
    }

    select {
    case p.conns <- conn:
        atomic.AddInt64(&p.stats.PutCount, 1)
    default:
        // 空闲队列满（size > MaxSize 已不可能，但防御性处理）
        conn.Close()
        p.mu.Lock()
        p.size--
        p.mu.Unlock()
    }
    return nil
}
```

### 后台健康检查与预热

```go
func (p *ConnectionPool) start() {
    // 预热：创建 MinSize 个连接
    for i := 0; i < p.opts.MinSize; i++ {
        conn, err := p.createConn()
        if err == nil {
            p.conns <- conn
        }
    }

    // 后台健康检查
    go func() {
        ticker := time.NewTicker(p.opts.HealthCheckInterval)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                p.healthCheck()
            case <-p.closeCh:
                return
            }
        }
    }()
}

func (p *ConnectionPool) healthCheck() {
    // 将空闲连接逐一取出检查，有效的放回，无效的关闭
    // 确保池中连接数不低于 MinSize
    for len(p.conns) > 0 {
        select {
        case conn := <-p.conns:
            if p.validator.IsValid(conn) {
                p.conns <- conn
            } else {
                conn.Close()
                p.mu.Lock()
                p.size--
                p.mu.Unlock()
            }
        default:
            goto done
        }
    }
done:
    // 补充到 MinSize
    p.mu.Lock()
    needed := p.opts.MinSize - p.size
    p.mu.Unlock()
    for i := 0; i < needed; i++ {
        conn, err := p.createConn()
        if err == nil {
            p.conns <- conn
        }
    }
}
```

## PoolStats（统计）

```go
type PoolStats struct {
    GetCount    int64 // 成功 Get 次数
    PutCount    int64 // 成功 Put 次数
    CreateCount int64 // 新建连接次数
    CloseCount  int64 // 关闭连接次数
    WaitCount   int64 // 等待可用连接次数（池已满时）
}

// 获取统计
stats := pool.Stats()
fmt.Printf("pool hits: %d, misses(creates): %d\n",
    stats.GetCount - stats.CreateCount, stats.CreateCount)
```

## PoolManager（多地址管理）

Discovery 模式下，Client 需要为多个服务实例（地址）分别维护连接池，`PoolManager` 使用双重检查锁模式管理这些池：

**源码**：`pkg/pool/pool_manager.go`（112 行）

```go
type PoolManager struct {
    pools  map[string]*ConnectionPool
    mu     sync.RWMutex
    opts   PoolOptions
    factory ConnectionFactory
}

func (m *PoolManager) GetConnection(address string) (*PooledConnection, error) {
    // 快速路径：读锁查找
    m.mu.RLock()
    pool, ok := m.pools[address]
    m.mu.RUnlock()

    if !ok {
        // 慢路径：写锁创建（双重检查）
        m.mu.Lock()
        if pool, ok = m.pools[address]; !ok {
            pool = NewConnectionPool(address, m.opts, m.factory)
            m.pools[address] = pool
        }
        m.mu.Unlock()
    }

    return pool.Get(context.Background())
}

func (m *PoolManager) RemovePool(address string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if pool, ok := m.pools[address]; ok {
        pool.Close()
        delete(m.pools, address)
    }
}

func (m *PoolManager) Stats() map[string]PoolStats {
    m.mu.RLock()
    defer m.mu.RUnlock()
    stats := make(map[string]PoolStats, len(m.pools))
    for addr, pool := range m.pools {
        stats[addr] = pool.Stats()
    }
    return stats
}
```

## 配置建议

| 场景 | MinSize | MaxSize | IdleTimeout |
|------|---------|---------|-------------|
| 低流量服务 | 2 | 10 | 60s |
| 中流量服务 | 5 | 50 | 90s |
| 高流量服务 | 10 | 100 | 120s |
| 极高流量（每实例）| 20 | 200 | 180s |

**连接复用率** = `(GetCount - CreateCount) / GetCount`，目标 > 95%。可通过调大 `MinSize` 和 `MaxSize` 提升复用率。

## 相关文档

- [TCP 传输](tcp.md) — PooledConnection 底层的 TCP 客户端
- [服务发现模式](../client/discovery-mode.md) — PoolManager 的使用场景
- [Client 概述](../client/overview.md)
