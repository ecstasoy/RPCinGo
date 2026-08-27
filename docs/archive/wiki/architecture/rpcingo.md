# 整体架构

## 分层模型

RPCinGo 采用清晰的五层架构，每层职责单一、依赖方向单一（上层依赖下层，下层不知道上层）：

```
┌─────────────────────────────────────────────────────────────┐
│                  应用层（User Code）                          │
│   client.Call() / client.CallTyped() / srv.RegisterService() │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│             RPC 核心层（pkg/client, pkg/server）              │
│   拦截器链 · 服务注册/路由 · 连接池管理 · 错误映射              │
│   服务发现 · 负载均衡 · 熔断 · 限流（Discovery 模式）          │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│              协议层（pkg/protocol）                           │
│   20 字节固定头 · Request · Response · Error · Metadata       │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│              编解码层（pkg/codec）                            │
│   JSON · Protobuf · Gzip 压缩装饰器 · 全局注册表              │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│              传输层（pkg/transport/tcp）                      │
│   TCP 客户端/服务端 · ProtocolCodec · 信号量并发控制           │
└─────────────────────────────────────────────────────────────┘
```

## 完整组件关系图

```
                    ┌──────────────┐
                    │  pkg/config  │
                    │ YAML → Options│
                    └──────┬───────┘
              ┌────────────┼────────────┐
              │            │            │
    ┌─────────▼──────┐  ┌──▼─────────┐ │
    │   pkg/server   │  │ pkg/client │ │
    │                │  │            │ │
    │ ServiceRegistry│  │ Fixed Mode │ │
    │ InterceptorChain  │ Discovery  │ │
    │ ErrorMapping   │  │ Mode       │ │
    └────────┬───────┘  └─────┬──────┘ │
             │                │        │
             │         ┌──────┼────────┘
             │         │      │
    ┌─────────▼─────────▼──┐  │
    │   pkg/interceptor     │  │
    │ Recovery·Log·Metrics  │  │
    │ RateLimit·Retry·CB    │  │
    └─────────┬─────────────┘  │
              │                │
    ┌─────────▼──────┐         │
    │  pkg/protocol  │         │
    │ Header·Req·Resp│         │
    │ Error·Metadata │         │
    └────────┬───────┘         │
             │                 │
    ┌────────▼────────┐        │
    │    pkg/codec    │        │
    │ JSON·Protobuf   │        │
    │ Gzip·Registry   │        │
    └────────┬────────┘        │
             │                 │
    ┌────────▼────────┐        │
    │  pkg/transport  │◄───────┘
    │      /tcp       │   pkg/pool
    │ Client·Server   │  连接池管理
    │ ProtocolCodec   │
    └─────────────────┘
             ▲
             │
    ┌────────┴────────────────────────────┐
    │           Discovery 模式组件         │
    │                                     │
    │ pkg/registry    pkg/loadbalancer     │
    │  etcd/memory    RR/Random/Weighted   │
    │  Registry       ConsistentHash       │
    │  Discovery                           │
    │  Watcher        pkg/circuitbreaker   │
    │                 三状态机+SlidingWindow│
    │                                     │
    │                 pkg/ratelimiter      │
    │                 TokenBucket          │
    │                 SlidingWindow        │
    └─────────────────────────────────────┘
```

## 核心包职责速览

| 包 | 代码行数 | 核心类型 | 职责 |
|----|---------|---------|------|
| `pkg/protocol` | ~350 | `Header`, `Request`, `Response`, `Error`, `Metadata` | 定义网络消息格式与错误体系 |
| `pkg/codec` | ~800 | `Codec`, `StreamCodec`, `CompressedCodec` | 序列化/反序列化与压缩 |
| `pkg/transport/tcp` | ~800 | `TCPClient`, `TCPServer`, `ProtocolCodec` | TCP 网络通信 |
| `pkg/server` | ~650 | `Server`, `ServiceRegistry` | RPC 服务端核心 |
| `pkg/client` | ~460 | `Client` | RPC 客户端核心 |
| `pkg/pool` | ~950 | `ConnectionPool`, `PoolManager` | 连接池管理 |
| `pkg/registry` | ~400 | `Registry`, `Discovery`, `Watcher` | 服务注册与发现接口 |
| `pkg/loadbalancer` | ~300 | 4 种算法 | 负载均衡 |
| `pkg/circuitbreaker` | ~350 | `CircuitBreaker`, `SlidingWindow` | 熔断保护 |
| `pkg/ratelimiter` | ~230 | `TokenBucket`, `SlidingWindow` | 流量控制 |
| `pkg/interceptor` | ~260 | `Interceptor`, `Chain` | 横切关注点 |
| `pkg/config` | ~223 | `Config`, Builder 函数 | YAML 配置加载 |

## 两种运行模式

### Fixed 模式（直连）

```
Application
    │ client.Call(service, method, args)
    ▼
Client
    │ pool.Get()
    ▼
ConnectionPool → TCPClient → TCP 连接
                                │
                                ▼
                         TCPServer
                                │ decodeRequest
                                ▼
                         InterceptorChain
                                │
                                ▼
                         ServiceRegistry.Invoke()
                                │
                                ▼
                         Handler(ctx, args) → (result, error)
```

**适用**：开发/测试、地址固定的单节点或已知拓扑。

### Discovery 模式（微服务）

```
Application
    │ client.Call(service, method, args)
    ▼
Client
    │ GetInstances(service) ←→ etcd（本地缓存）
    │ LoadBalancer.Pick()
    │ CircuitBreaker.Allow()
    │ PoolManager.GetConnection(address)
    ▼
ConnectionPool → TCPClient → TCP 连接
    [后台 Watch goroutine 维护实例列表最新]
    [每实例独立熔断器，互不干扰]
```

**适用**：生产微服务、多实例部署、动态扩缩容。

## 可观测性

```
Server InterceptorChain
    │
    ├── MetricsInterceptor
    │   └── rpc_calls_total (Counter)
    │   └── rpc_duration_seconds (Histogram)
    │
    ├── LoggingInterceptor
    │   └── 结构化日志（含 trace-id）
    │
    └── RecoveryInterceptor
        └── panic → error（不崩溃）

连接池统计：
    PoolStats {GetCount, PutCount, CreateCount, CloseCount}
```

## 相关文档

- [数据流](data-flow.md) — 端到端请求处理链路
- [设计模式](design-patterns.md) — 框架核心设计模式
- [Server 概述](../server/overview.md)
- [Client 概述](../client/overview.md)


# 数据流

## Fixed 模式完整请求链路

```
应用代码
    │
    │  cli.Call(ctx, "UserService", "GetUser", args)
    ▼
─────────────────────────── pkg/client ─────────────────────────────
    │
    │  1. 构建 Request：
    │     ID      = atomic.AddUint64(&globalID, 1)  // 全局原子自增
    │     Service = "UserService"
    │     Method  = "GetUser"
    │     Args    = args                              // 原始 Go 对象
    │     Timeout = callTimeout.Milliseconds()
    │     CreatedAt = time.Now().UnixMilli()
    │     Metadata = ctx 中的 metadata（如 trace-id）
    │
    │  2. 执行客户端拦截器链（前置）
    │     → Retry → Logging → ...
    │
    │  3. pool.Get(ctx)  // 从连接池获取 TCP 连接
    │
─────────────────────────── pkg/codec ──────────────────────────────
    │
    │  4. codec.Encode(request)  // 序列化 Request 结构体
    │     JSON: json.Marshal(request)  → []byte（如 "{"id":42,...}"）
    │
    │  5. compressor.Compress(body)  // 可选 Gzip 压缩
    │
─────────────────────────── pkg/transport/tcp ──────────────────────
    │
    │  6. 构建 Header（20 字节）：
    │     [0:2]   Magic = 0xCAFE
    │     [2]     Version = 1
    │     [3]     MsgType = 1（Request）
    │     [4]     Codec = 1（JSON）
    │     [5]     Compress = 0（None）
    │     [6:8]   Reserved = 0x0000
    │     [8:16]  RequestID = 42（big-endian）
    │     [16:20] BodyLength = 156（big-endian）
    │
    │  7. conn.Write([Header(20B) | Body(156B)])
    │     TCP 无延迟（NoDelay=true），立即发送
    │
    │═══════════════ 网络传输 ═══════════════════
    │
─────────────────────────── pkg/transport/tcp (服务端) ─────────────
    │
    │  8. io.ReadFull(conn, headerBuf[:20])   // 精确读取 20 字节 Header
    │  9. decodeHeader(headerBuf)             // 验证 Magic=0xCAFE，获取 BodyLength=156
    │
    │  10. io.ReadFull(conn, bodyBuf[:156])   // 按 BodyLength 读取 Body
    │
    │  11. compressor.Decompress(bodyBuf)    // 解压（若 Header.Compress != None）
    │
─────────────────────────── pkg/codec ──────────────────────────────
    │
    │  12. codec.Decode(bodyBuf, &request)   // 反序列化 Body → Request 对象
    │
─────────────────────────── pkg/server ─────────────────────────────
    │
    │  13. 执行服务端拦截器链（前置 → 后置）：
    │      Recovery → Logging → Metrics → RateLimit → [Handler] → RateLimit → Metrics → Logging → Recovery
    │
    │  14. ServiceRegistry.Invoke(ctx, request)：
    │      key = "UserService.GetUser"
    │      handler = registry.services["UserService"].methods["GetUser"].handler
    │
    │  15. 反序列化 Args → *UserRequest（根据 ArgsCodec 选择 JSON/Protobuf）
    │
    │  16. handler(ctx, *UserRequest)  // 反射调用
    │      → return (*UserResponse, nil)
    │
    │  17. 构建 Response：
    │      ID        = request.ID（= 42，用于匹配）
    │      Data      = *UserResponse
    │      Error     = nil
    │      ServerTime = time.Now().UnixMilli()
    │
─────────────────────────── pkg/codec ──────────────────────────────
    │
    │  18. codec.Encode(response) → []byte
    │  19. compressor.Compress(body) // 可选
    │
─────────────────────────── pkg/transport/tcp ──────────────────────
    │
    │  20. 构建 Response Header（20 字节，MsgType=2）
    │  21. conn.Write([Header | Body])
    │
    │═══════════════ 网络传输 ═══════════════════
    │
─────────────────────────── pkg/client ─────────────────────────────
    │
    │  22. ReadResponse()：ReadFull(Header) + ReadFull(Body)
    │  23. codec.Decode(body, &response)
    │  24. unmapError(response.Error) → Go error（client/error_map.go）
    │  25. pool.Put(conn)          // 归还连接
    │  26. 执行客户端拦截器链（后置）
    │
    ▼
应用代码接收 (result, error)
```

## Discovery 模式附加步骤

在步骤 3（pool.Get）之前，Discovery 模式额外执行：

```
步骤 3 之前：

    3a. c.instancesMu.RLock()
        instances = c.instances（本地缓存，由 Watch goroutine 维护）
        c.instancesMu.RUnlock()

    3b. loadBalancer.Pick(instances)
        → 选中 instance{Address:"10.0.0.1", Port:8080}

    3c. breaker = c.getBreaker("10.0.0.1:8080")
        if !breaker.Allow() {
            return nil, ErrServiceUnavailable  // 熔断，直接返回
        }

    3d. pool = c.poolManager.GetPool("10.0.0.1:8080")
        conn = pool.Get(ctx)

步骤 25 之后：

    25a. if err != nil {
             breaker.RecordFailure()  // 失败计数
         } else {
             breaker.RecordSuccess()  // 成功计数
         }
```

## 后台 Watch goroutine（Discovery 模式）

与请求链路并行运行的后台 goroutine：

```
NewDiscoveryClient()
    │
    └── go watchInstances():
            watcher, _ := discovery.Watch(ctx, serviceName)
            for {
                event := watcher.Next()  // 阻塞等待 etcd 事件
                switch event.Type {
                case EventAdd:
                    instancesMu.Lock()
                    instances = append(instances, event.Instance)
                    instancesMu.Unlock()

                case EventDelete:
                    instancesMu.Lock()
                    instances = removeByID(instances, event.Instance.ID)
                    instancesMu.Unlock()
                    poolManager.RemovePool(event.Instance.Address)  // 清理连接池

                case EventUpdate:
                    instancesMu.Lock()
                    updateInSlice(instances, event.Instance)
                    instancesMu.Unlock()
                }
            }
```

## 错误传播路径

```
Handler 返回错误
    │
    │ error_map.go (server)
    ▼
protocol.Error{Code: X, Message: "..."}
    │ 写入 Response.Error 字段
    │ 序列化 + 网络传输
    ▼
protocol.Error{Code: X, Message: "..."}
    │ error_map.go (client)
    ▼
Go error（context.DeadlineExceeded / ErrServiceUnavailable / 等）
    │
    ▼
CircuitBreaker.RecordFailure()  // 失败计数驱动熔断
```

## 关键数据类型流转

```
应用层对象（Go struct / proto.Message）
    │ json.Marshal / proto.Marshal
    ▼
[]byte（序列化 payload）
    │ 可选：gzip.Compress
    ▼
[]byte（压缩后 payload）
    │ 加 20 字节 Header
    ▼
[]byte = [Header(20B) | Body(N B)]  ← TCP 实际传输内容
    │ io.ReadFull × 2（先 Header，再 Body）
    ▼
[]byte（压缩后 payload）
    │ 可选：gzip.Decompress
    ▼
[]byte（序列化 payload）
    │ json.Unmarshal / proto.Unmarshal
    ▼
应用层对象（Go struct / proto.Message）
```

## 相关文档

- [协议头](../protocol/header.md) — Header 字段详解
- [TCP 传输](../transport/tcp.md) — 两阶段读取实现
- [Codec 概述](../codec/overview.md) — 序列化流程
- [拦截器链](../server/interceptors.md) — 拦截器执行顺序
- [熔断器](../reliability/circuit-breaker.md) — Allow/Record 方法


# 设计模式

RPCinGo 一致地使用若干 Go 惯用设计模式。理解这些模式能帮助你快速读懂任意模块的代码，也能快速扩展框架。

## 1. Options 模式（函数式选项）

**用途**：为结构体提供灵活、可扩展的配置，避免大量位置参数，零值有合理默认值。

**实现位置**：`pkg/server/options.go`、`pkg/client/options.go`、`pkg/pool/pool.go` 等

```go
// 选项函数类型
type Option func(*serverOptions)

// 选项实现
func WithAddress(addr string) Option {
    return func(o *serverOptions) {
        o.address = addr
    }
}

func WithCodec(codec protocol.CodecType, compress protocol.CompressType) Option {
    return func(o *serverOptions) {
        o.codec = codec
        o.compress = compress
    }
}

// 使用：可组合，可选，顺序无关
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
    server.WithTimeout(5*time.Second, 5*time.Second),
    server.WithMaxConcurrent(1000),
)
```

**优点**：新增配置项不需要修改调用方；内部 `serverOptions` 结构体私有，不暴露实现细节。

---

## 2. Registry 模式（全局注册表）

**用途**：Codec 通过类型 ID 注册到全局单例，运行时按类型查找，实现可插拔的序列化机制。

**实现位置**：`pkg/codec/codec.go`、`pkg/codec/compress.go`

```go
// 注册表内部结构（私有）
var defaultRegistry = &codecRegistry{
    codecs: make(map[protocol.CodecType]Codec),
}

// 公开注册函数
func Register(t protocol.CodecType, c Codec) { ... }
func Get(t protocol.CodecType) Codec { ... }

// 各 Codec 在 init() 中自动注册
// pkg/codec/json.go
func init() { Register(protocol.CodecTypeJSON, &JSONCodec{}) }

// pkg/codec/protobuf.go
func init() { Register(protocol.CodecTypeProtobuf, &ProtobufCodec{}) }

// 使用：接收到网络数据后，按 Header.Codec 取对应 Codec
c := codec.Get(header.Codec)  // 运行时按类型查找
c.Decode(body, &request)
```

**扩展**：添加自定义 Codec 只需在应用层调用 `codec.Register(CodecTypeMsgpack, myCodec)`，无需修改框架代码。

---

## 3. 装饰器模式（压缩 Codec）

**用途**：将 Gzip 压缩能力透明叠加在任意 Codec 之上，不修改原有 Codec 实现。

**实现位置**：`pkg/codec/codec.go` — `CompressedCodec`

```go
// CompressedCodec 既是 Codec（实现 Encode/Decode）
// 又包含一个 inner Codec 和 Compressor
type CompressedCodec struct {
    inner      Codec
    compressor Compressor
}

// Encode = inner.Encode + Compress
func (c *CompressedCodec) Encode(v interface{}) ([]byte, error) {
    data, err := c.inner.Encode(v)
    if err != nil { return nil, err }
    return c.compressor.Compress(data)
}

// Decode = Decompress + inner.Decode
func (c *CompressedCodec) Decode(data []byte, v interface{}) error {
    decompressed, err := c.compressor.Decompress(data)
    if err != nil { return err }
    return c.inner.Decode(decompressed, v)
}
```

可以多层嵌套（虽然实际不常用）：`CompressedCodec{inner: CompressedCodec{inner: JSONCodec}}`

---

## 4. 中间件/拦截器链模式

**用途**：将日志、监控、限流、熔断、恢复等横切关注点以链式组合，各自独立、顺序执行、可复用。

**实现位置**：`pkg/interceptor/interceptor.go`

```go
type Interceptor func(ctx context.Context, req *protocol.Request, next Invoker) (interface{}, error)

// 链构建：从后往前嵌套，形成洋葱结构
func (c *Chain) Execute(ctx context.Context, req *protocol.Request, final Invoker) (interface{}, error) {
    h := final  // 最内层：实际 handler
    for i := len(c.interceptors) - 1; i >= 0; i-- {
        next, interceptor := h, c.interceptors[i]
        h = func(ctx context.Context, req *protocol.Request) (interface{}, error) {
            return interceptor(ctx, req, next)
        }
    }
    return h(ctx, req)
}
```

注册顺序 `[A, B, C]` 的执行顺序：

```
请求: A前 → B前 → C前 → Handler → C后 → B后 → A后
```

---

## 5. 工厂模式（连接池）

**用途**：将 TCP 连接的创建逻辑与连接池本身解耦，便于测试（注入 mock 工厂）和替换传输实现。

**实现位置**：`pkg/pool/pool.go`

```go
type ConnectionFactory interface {
    Create(address string) (*tcp.Client, error)
}

// 内置工厂：直接建立 TCP 连接
type DefaultConnectionFactory struct{ opts ClientOptions }

// 内置工厂：失败时指数退避重试
type RetryConnectionFactory struct {
    inner      ConnectionFactory
    maxRetries int
    baseDelay  time.Duration
}

// 连接池接受注入的工厂
pool := NewConnectionPool(address, opts,
    WithFactory(&RetryConnectionFactory{
        inner:      &DefaultConnectionFactory{},
        maxRetries: 3,
        baseDelay:  100 * time.Millisecond,
    }),
)
```

---

## 6. 反射注册模式（服务自动发现）

**用途**：服务端无需为每个方法手动注册 handler，通过反射自动提取符合签名的公共方法，降低样板代码。

**实现位置**：`pkg/server/service.go`

```go
// 一行注册 CalculatorService 的所有方法
srv.RegisterService("Calculator", &CalculatorService{})
// 框架自动发现并注册：Calculator.Add, Calculator.Subtract

// 内部实现
for i := 0; i < reflect.TypeOf(impl).NumMethod(); i++ {
    method := reflect.TypeOf(impl).Method(i)
    if !method.IsExported() { continue }

    handler, reqType, ok := makeHandler(reflect.ValueOf(impl), method)
    if !ok { continue } // 签名不匹配，跳过

    svc.methods[method.Name] = &MethodInfo{handler, reqType}
}
```

---

## 7. 状态机模式（熔断器）

**用途**：熔断器的三个状态（Closed/Open/HalfOpen）及其转换条件，用显式状态机建模，逻辑清晰，易于测试和调试。

**实现位置**：`pkg/circuitbreaker/`

```go
func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case StateClosed:
        return true  // 正常状态：直接放行

    case StateOpen:
        if time.Now().After(cb.expiry) {
            cb.toHalfOpen()  // 超时：进入探测
            return true
        }
        return false  // 熔断中：拒绝

    case StateHalfOpen:
        cb.halfOpenRequests++
        return cb.halfOpenRequests <= cb.config.MaxRequests
    }
    return false
}
```

每个状态转换都是明确的方法（`toOpen`、`toHalfOpen`、`toClosed`），便于在测试中直接触发和验证。

---

## 8. 双重检查锁（PoolManager）

**用途**：在并发访问时，按需为新服务实例地址创建连接池，避免重复创建，同时最小化锁持有时间。

**实现位置**：`pkg/pool/pool_manager.go`

```go
func (m *PoolManager) GetPool(address string) *ConnectionPool {
    // 快速路径：读锁
    m.mu.RLock()
    pool, ok := m.pools[address]
    m.mu.RUnlock()
    if ok {
        return pool
    }

    // 慢路径：写锁 + 再次检查（Double-Checked Locking）
    m.mu.Lock()
    defer m.mu.Unlock()
    if pool, ok = m.pools[address]; !ok {
        pool = NewConnectionPool(address, m.opts, m.factory)
        m.pools[address] = pool
    }
    return pool
}
```

"双重检查"防止在两个 goroutine 都通过第一次 RLock 检查后，重复创建同一地址的连接池。

---

## 9. 惰性补充模式（令牌桶限流）

**用途**：令牌桶不用定时器定期补充，而是在每次 `Allow()` 调用时惰性计算应补充的令牌数，节省 goroutine 和定时器资源。

**实现位置**：`pkg/ratelimiter/token_bucket.go`

```go
func (l *TokenBucketLimiter) AllowN(n int) bool {
    l.mu.Lock()
    defer l.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(l.last)

    // 惰性计算：自上次调用以来应补充多少令牌
    elapsedNs := elapsed.Nanoseconds() + l.remainderNs
    newTokens := float64(elapsedNs) * l.rate / 1e9
    l.remainderNs = elapsedNs - int64(newTokens/l.rate*1e9)

    l.tokens = math.Min(l.tokens+newTokens, l.capacity)
    l.last = now
    // ...
}
```

无论调用频率如何，令牌补充速率始终精确等于配置的 rate（纳秒余量保证精度）。

---

## 模式与包的对应关系

| 设计模式 | 实现包 |
|----------|--------|
| Options 模式 | `pkg/server`, `pkg/client`, `pkg/pool`, `pkg/transport` |
| Registry 模式 | `pkg/codec` |
| 装饰器模式 | `pkg/codec` (`CompressedCodec`) |
| 中间件/拦截器链 | `pkg/interceptor` |
| 工厂模式 | `pkg/pool` (`ConnectionFactory`) |
| 反射注册 | `pkg/server` (`ServiceRegistry`) |
| 状态机 | `pkg/circuitbreaker` |
| 双重检查锁 | `pkg/pool` (`PoolManager`) |
| 惰性补充 | `pkg/ratelimiter` (`TokenBucketLimiter`) |

## 相关文档

- [Codec 概述](../codec/overview.md) — Registry 与装饰器实现
- [拦截器链](../server/interceptors.md) — 中间件实现
- [熔断器](../reliability/circuit-breaker.md) — 状态机实现
- [连接池](../transport/connection-pool.md) — 工厂与双重检查锁


