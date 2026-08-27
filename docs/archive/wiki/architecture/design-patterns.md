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
