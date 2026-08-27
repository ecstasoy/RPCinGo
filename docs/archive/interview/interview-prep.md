# 简历项目面试准备手册

> 对应简历版本 A，按四个 bullet 逐条拆解全流程 + 可能的面试问题

---

## Bullet 1 — 自定义二进制协议 · 编解码 · 压缩 · 帧读写

### 全流程

#### 第一层：协议设计（Header）

一条 RPC 消息由两部分组成：**20 字节定长 Header** + **变长 Body**。

```
Byte:  0  1  2  3  4  5  6  7  8  9  10 11 12 13 14 15 16 17 18 19
      ┌──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┐
      │Magic │Ver│Typ│Cod│Cmp│Reserv │       RequestID       │BodyLen│
      └──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┘
```

| 字段 | 字节 | 作用 |
|------|------|------|
| Magic `0xCAFE` | 0-1 | 魔数，接收方首先校验，防止把其它协议的数据当 RPC 消息处理 |
| Version `0x01` | 2 | 协议版本，不匹配时拒绝连接，保证升级兼容性 |
| MsgType | 3 | `0x01` Request / `0x02` Response，区分消息方向 |
| Codec | 4 | `0x00` JSON / `0x01` Protobuf，告诉接收方 Body 如何反序列化 |
| Compress | 5 | `0x00` None / `0x01` Gzip，告诉接收方是否需要先解压 |
| Reserved | 6-7 | 保留，全 0，为未来扩展预留 |
| RequestID | 8-15 | uint64，原子自增，用于请求/响应匹配 |
| BodyLength | 16-19 | uint32，Body 的字节数，读取时知道要读多少 |

全部使用**大端字节序（Big Endian）**，网络字节序的标准做法。

#### 第二层：序列化（Codec）

Codec 层负责把 `*protocol.Request` / `*protocol.Response` 序列化成 `[]byte`，与传输层解耦。

```
接口：Encode(v interface{}) ([]byte, error)
      Decode(data []byte, v interface{}) error

注册表：map[CodecType]Codec，由 sync.RWMutex 保护
        Get(typ)：找不到返回 nil
        GetOrDefault(typ)：找不到返回 JSON
```

实现：
- **jsonCodec**：`encoding/json`，调试友好，Body 可人眼阅读
- **protobufCodec**：`google.golang.org/protobuf/proto`，更小、更快，强类型

#### 第三层：压缩（Compressor）

压缩是可选的装饰器，`CompressedCodec` 包裹任意 `Codec`：

```
发送：codec.Encode(req) → bodyBytes → compressor.Compress(bodyBytes) → wire
接收：wire → compressor.Decompress → bodyBytes → codec.Decode(bodyBytes, &req)
```

Header 中的 `Compress` 字段由发送方写入，接收方根据该字段选择解压器，**不要求双方配置相同**（服务端兼容不压缩的客户端）。

#### 第四层：帧读写（ProtocolCodec）

`tcp/codec.go` 中的 `ProtocolCodec` 直接操作 `net.Conn`，处理 TCP 流的帧边界问题。

**发送一条请求的完整步骤：**

```
EncodeRequest(req):
  1. codec.Encode(req)          → bodyBytes（序列化）
  2. compressor.Compress(body)  → compressedBytes（压缩，可能 no-op）
  3. NewHeader(MsgTypeRequest, codecType, req.ID, len(compressedBytes))
  4. header.Encode()            → 20 字节
  5. result = header[20] + compressedBytes
  6. conn.Write(result)         → 写入 TCP 流
```

**接收一条响应的完整步骤（`DecodeFromReader`）：**

```
1. io.ReadFull(conn, [20]byte)     → 精确读 20 字节，得到 Header
2. header.Decode(bytes)            → 校验 Magic、Version，提取 BodyLength
3. io.ReadFull(conn, [BodyLength]byte) → 精确读 Body（防止半包）
4. compressor.Decompress(body)     → 按 header.Compress 字段选解压器
5. 返回 header + decompressedBytes
```

`io.ReadFull` 是解决 TCP 粘包/拆包的关键：它会一直阻塞直到读满指定字节数，而不会返回"读了一半"的数据。

---

### 可能的面试问题

**Q1：为什么 Header 选 20 字节，不多不少？**

A：字段分析：2（Magic）+ 1（Version）+ 1（Type）+ 1（Codec）+ 1（Compress）+ 2（Reserved）= 8 字节，8（RequestID）+ 4（BodyLength）= 12 字节，合计 20 字节。每个字段都有意义，Reserved 留了 2 字节供未来扩展而不破坏对齐。固定长度的好处是接收方永远知道"先读 20 字节"，无需额外的长度前缀标识头部本身的大小。

**Q2：Magic Number `0xCAFE` 有什么用？能不要吗？**

A：不能去掉。TCP 是字节流协议，两端建立连接后如果一端发了错误的数据（例如 HTTP 请求误连到 RPC 端口），没有 Magic 的话服务端会尝试把乱数据当 Header 解析，导致难以定位的崩溃。有 Magic 后，解析 Header 的第一步就是校验 `buf[0:2] == 0xCAFE`，不匹配直接关闭连接，隔离了非法连接。gRPC 的 HTTP/2 帧也有类似的 magic bytes（`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`）。

**Q3：为什么用大端字节序？**

A：网络标准（RFC 791 等）规定网络字节序为大端，`binary.BigEndian` 是 Go 标准库提供的直接支持。小端是 x86 CPU 的本地字节序，如果用小端，在 ARM 服务器或跨平台通信时需要额外转换。大端在网络协议中是惯例，可读性也更好（高位字节在前）。

**Q4：JSON 和 Protobuf 各自的优缺点，你的框架怎么选？**

A：

| 维度 | JSON | Protobuf |
|------|------|----------|
| 体积 | 大（字段名作为 key 传输） | 小（用字段编号，无字段名） |
| 速度 | 慢（字符串解析） | 快（二进制解析） |
| 可读性 | 人眼可读，调试方便 | 二进制，需要 proto 文件才能解码 |
| 类型安全 | 弱（number/string 容易混淆） | 强（由 .proto 文件约束） |
| 向后兼容 | 需要手动处理 | 字段编号机制天然兼容 |

框架通过 Header 中的 Codec 字节做运行时切换，测试/调试用 JSON，生产用 Protobuf。

**Q5：TCP 是字节流，你怎么解决粘包和拆包？**

A：用**定长 Header + 长度字段**的方案。Header 固定 20 字节，其中 `BodyLength` 字段明确告知 Body 大小。读取时分两次 `io.ReadFull`：第一次精确读 20 字节解析 Header，第二次按 `BodyLength` 精确读 Body。`io.ReadFull` 内部会循环调用 `Read` 直到凑满指定字节数，不会返回半包。这也是 Kafka、Redis RESP 协议的通用思路。

**Q6：Gzip 压缩在什么场景下有收益，什么场景下反而更慢？**

A：Gzip 对**文本类数据**（JSON、大字符串）压缩率高（可达 70%+），减少网络传输时间，在带宽受限或跨机房场景下有明显收益。但 Gzip 本身有 CPU 开销，对**小包**（< 1KB）或**已压缩数据**（图片、视频）反而会增大体积并消耗 CPU，得不偿失。生产中通常只对 Body > 某阈值（比如 4KB）的请求启用压缩。

---

## Bullet 2 — TCP 连接池 · etcd 服务发现 · 两种客户端模式

### 全流程

#### 连接池核心结构

```go
type ConnectionPool struct {
    address     string
    opts        *PoolOptions        // 配置参数
    pool        chan *PooledConnection  // 核心：channel 当容器
    factory     ConnectionFactory   // 创建连接的工厂
    currentSize int                 // 当前总连接数（含在用的）
    mu          sync.RWMutex        // 保护 currentSize 和 closed
}
```

**为什么用 channel 而不是 slice + mutex？**

channel 天然支持非阻塞的 `select/default`：

```go
// Get() 的快路径（无锁）
select {
case conn := <-p.pool:   // 有空闲连接：直接取
    if conn.IsHealthy() && !conn.IsExpired(...) {
        return conn, nil
    }
    // 不健康则关闭，继续走慢路径
default:                 // 池空：创建新连接
    return p.createNewConnectionWithContext(ctx)
}
```

`Put()` 归还时也类似：

```go
select {
case p.pool <- conn:  // 放回池中
default:              // 池满：直接关闭连接，不阻塞
    conn.Close()
}
```

channel 的容量 = MaxSize，同时限制了池的上限，不需要额外的计数逻辑。

#### 连接池生命周期

```
NewConnectionPool()
  ├── 校验 options（DefaultPoolValidator）
  ├── 预创建 MinSize 条连接放入 pool channel
  ├── go cleanupRoutine()     每 CleanupInterval 清理过期连接，补足 MinSize
  └── go healthCheckRoutine() 每 HealthCheckInterval 逐一检查连接可用性

Get() → 业务使用 → conn.Release() → Put() 归还

Close()
  ├── close(stopCleanup) 停止后台 goroutine
  ├── close(pool channel)
  └── 逐一关闭 channel 中剩余连接
```

#### PooledConnection 的过期判断

```go
func (pc *PooledConnection) IsExpired(maxIdleTime, maxLifetime time.Duration) bool {
    // 规则1：超过 maxIdleTime 未被使用（空闲超时，默认 90s）
    if now.Sub(pc.lastUsed) > maxIdleTime { return true }
    // 规则2：连接存活超过 maxLifetime（全生命周期，默认 30min，防止长连接积累问题）
    if maxLifetime > 0 && now.Sub(pc.createdAt) > maxLifetime { return true }
    return false
}
```

#### PoolManager（多地址）

服务发现模式下，一个服务可能有多个实例（不同 IP:Port）。`PoolManager` 以地址为 key 管理一组 `ConnectionPool`：

```go
GetConnection(ctx, "192.168.1.1:8080")
  → 读锁查找 pools["192.168.1.1:8080"]
  → 存在 → 直接 Get()
  → 不存在 → 升级写锁 → double-check → 创建新 Pool → 存入 map → Get()
```

Double-checked locking 避免多个 goroutine 同时为同一地址创建池。当服务实例下线时，Watch 事件触发 `RemovePool(addr)`，Close 该地址的连接池并从 map 删除。

#### etcd 服务注册流程

```
Server.Start()
  └── registerService()
        ├── NewServiceInstance(name, host, port)   → ID = "service-host:port-unix"
        ├── EtcdRegistry.Register()
        │     └── client.Put("/rpc/services/UserService/UserService-127.0.0.1:8080-1234567",
        │                    json(instance), withLease(leaseID))
        └── go startHeartbeat()  每 5s 调用 KeepAliveOnce(leaseID)

Stop()
  ├── close(stopHeartbeat)
  └── EtcdRegistry.Deregister()  → client.Delete(key)
```

etcd 租约机制：Server 启动时申请一个 TTL=10s 的租约，所有 key 都绑定这个租约。如果进程崩溃，心跳停止，10s 后租约自动过期，key 被 etcd 自动删除——**客户端会通过 Watch 感知到服务下线，无需服务端主动通知**。

#### 客户端两种模式

**固定地址模式（Direct）：**

```
NewClient("127.0.0.1:8080")
  └── 创建单个 ConnectionPool
      Call() → fixedPool.Get() → conn.SendRequest() → fixedPool.Put()
```

**服务发现模式（Discovery）：**

```
NewDiscoveryClient(WithDiscovery(etcdDiscovery), ...)
  └── 创建 PoolManager（空）

Call("UserService", "GetUser", args)
  ├── getInstances("UserService")
  │     ├── 有缓存 → 直接返回
  │     └── 无缓存 → etcd.GetInstances() → 写入 instanceCache
  │                → go watchService("UserService")（后台监听变更）
  ├── loadBalancer.Pick(instances)  → 选一个 ServiceInstance
  ├── poolManager.GetConnection(instance.Endpoint())  → 按地址取连接
  └── conn.SendRequest()
```

Watch 事件处理：

```
etcd 变更事件 → EtcdWatcher.Next() → handleWatchEvent()
  case Add:    instanceCache[service] = append(...)
  case Delete: instanceCache[service] = 过滤掉该 ID
               poolManager.RemovePool(endpoint)   ← 及时释放连接
  case Update: instanceCache[service][i] = 新实例
```

---

### 可能的面试问题

**Q1：连接池为什么用 channel 而不是 sync.Pool？**

A：`sync.Pool` 是为**临时对象复用**设计的，GC 时会随时清空，不适合管理需要保活的 TCP 连接。channel 能精确控制容量上限（cap = MaxSize），提供非阻塞的 `select/default` 语义，并且可以让 goroutine 在池满时阻塞等待，这些是 sync.Pool 没有的能力。

**Q2：`Get()` 里有没有并发问题？currentSize 的修改是否线程安全？**

A：有一个 TOCTOU（time-of-check to time-of-use）弱点：检查 `currentSize < MaxSize` 和执行 `currentSize++` 之间没有原子保证，高并发下可能短暂超出 MaxSize 一点。实际影响有限，因为超出后多余的连接在 `Put()` 时会因 channel 满而被关闭。更严格的做法是用 `atomic.Int64` 或把 check+create 放在同一个锁区间内。这是个可以在面试里主动提出来的点，体现你读了代码细节。

**Q3：etcd 租约 TTL 为什么设 10s？太短或太长有什么问题？**

A：TTL 是服务注册的"死亡超时"——进程崩溃后，客户端最多等 TTL 秒才感知到服务下线。太短（< 3s）：心跳频率需要很高（< 1s/次），网络抖动容易误触发租约过期，造成服务"闪断"。太长（> 30s）：进程崩溃后客户端长时间向失效地址发请求，错误堆积。10s 是常见的工程折中，配合 5s 心跳间隔有足够的容错余量。

**Q4：Watch 机制和轮询有什么区别？**

A：轮询（polling）每隔固定时间主动查询 etcd，有延迟且对 etcd 有持续压力。Watch 是 etcd 的推送机制，基于 gRPC streaming，服务端变更时立即推送给客户端，延迟接近零（通常 < 10ms），且不产生冗余请求。Watch 连接本身也需要维护（断线重连），etcd 客户端库已处理了这部分逻辑。

**Q5：服务实例缓存没有 TTL，会有什么问题？**

A：这是一个已知缺陷。缓存首次填充后只依赖 Watch 事件更新。如果 Watch goroutine 因网络断开退出，缓存不会重新拉取，客户端会一直用陈旧的实例列表，可能向已下线的服务发请求。改进方案：在 Watch goroutine 退出时将缓存标记为 stale，下次 `getInstances()` 时强制回源 etcd 拉取。

**Q6：连接池的 MinSize 有什么用？**

A：MinSize 保证池里始终有一批"热"连接。没有 MinSize 的话，流量低谷时连接被清理干净，下一次流量高峰到来时需要重新建立大量 TCP 连接（三次握手有延迟），导致延迟尖刺。MinSize 是用内存换延迟稳定性的取舍，适合对延迟敏感的服务。

---

## Bullet 3 — 拦截器链 · Recovery · Logging · Metrics · RateLimit · Retry

### 全流程

#### 拦截器的类型定义

```go
type Invoker     func(ctx context.Context, req *protocol.Request) (any, error)
type Interceptor func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error)
```

每个 Interceptor 接收一个 `invoker`（下一层），可以在调用前后插入逻辑，形成洋葱模型。

#### Chain 的构建（buildChain）

```go
// NewChain(A, B, C) 的构建过程（反向迭代）：
// 初始 invoker = realHandler

// i=2: invoker = func(...) { return C(ctx, req, realHandler) }
// i=1: invoker = func(...) { return B(ctx, req, prev)         }
// i=0: invoker = func(...) { return A(ctx, req, prev)         }

// 最终调用顺序：A → B → C → realHandler → C.after → B.after → A.after
```

这是**闭包链式包裹**，不用递归，没有额外的栈开销。

#### 各拦截器工作机制

**Recovery（panic 保护）：**

```go
defer func() {
    if r := recover(); r != nil {
        stack := debug.Stack()
        err = fmt.Errorf("panic recovered: %v\nstack:\n%s", r, stack)
        resp = nil
    }
}()
return invoker(ctx, req)
```

捕获 handler 中的任意 panic，将其转为 error 返回给调用方，防止 goroutine 崩溃导致整个服务进程退出。必须是**第一个注册的拦截器**（最外层），才能覆盖所有内层的 panic。

**Logging：**

```go
service, method := req.Service, req.Method   // 从 req 读取，不是局部变量
logger.Infof("→ RPC call: [%s.%s]", service, method)

resp, err := invoker(ctx, req)

duration := time.Since(start)
if err != nil {
    logger.Errorf("✗ [%s.%s] failed in %v: %v", service, method, duration, err)
} else {
    logger.Infof("✓ [%s.%s] succeeded in %v", service, method, duration)
}
```

Logger 接口允许注入自定义实现（zap、logrus 等），默认降级为 `fmt.Printf`。

**Metrics（Prometheus）：**

```go
var rpcCallsTotal = prometheus.NewCounterVec(..., []string{"service","method","status"})
var rpcDuration   = prometheus.NewHistogramVec(..., []string{"service","method"})

// 在 init() 中注册到默认注册表

service, method := req.Service, req.Method
resp, err := invoker(ctx, req)

rpcCallsTotal.WithLabelValues(service, method, status).Inc()
rpcDuration.WithLabelValues(service, method).Observe(duration)
```

暴露的指标：
- `rpc_calls_total{service, method, status}` — Counter，用于计算 QPS 和错误率
- `rpc_duration_seconds{service, method}` — Histogram，用于计算 P99 延迟

**RateLimit（限流）：**

```go
if !limiter.Allow(ctx) {
    return nil, ErrRateLimitExceeded  // 直接拒绝，不调用 invoker
}
return invoker(ctx, req)
```

两种限流器：
- **令牌桶**：以恒定速率补充令牌，允许短暂突发流量（桶满时积累），适合接口限速
- **滑动窗口**：统计最近 N 秒内的请求数，硬性限制，无突发余量，适合防爬虫

**Retry（重试，客户端专用）：**

```go
for attempt := 0; attempt <= maxRetries; attempt++ {
    if attempt > 0 {
        select {
        case <-ctx.Done(): return nil, ctx.Err()   // 超时取消优先
        case <-time.After(interval):               // 等待间隔
        }
    }
    result, err := invoker(ctx, req)
    if err == nil { return result, nil }
    if !isRetryable(err) { return nil, err }       // 不可重试直接返回
    lastErr = err
}
return nil, lastErr
```

**可重试 vs 不可重试的判断（isRetryable）：**

```go
// protocol.Error 类型时，只重试基础设施级错误
case Unavailable, DeadlineExceeded, ResourceExhausted → 重试
case NotFound, InvalidArgument, PermissionDenied     → 不重试

// 非 protocol.Error（网络/IO 错误）→ 重试
```

**Retry 放在最外层的原因：** Retry 包裹整个内层链意味着 Logging 会记录每次重试的耗时，便于排查"第1次失败、第2次成功"的情况。如果 Retry 放内层，Logging 只会看到一次调用。

#### 服务端完整链路

```
HandleRequest(ctx, req)
  └── interceptor.Chain.Intercept(ctx, req, invoker)
        Recovery.before
          └── Logging.before
                └── Metrics.before
                      └── RateLimit（超限直接返回）
                            └── invoker()
                                  └── ServiceRegistry.GetHandler(service, method)
                                        └── reflect 调用业务方法
                      Metrics.after（记录耗时和状态）
                Logging.after（打印成功/失败日志）
          Recovery.after（捕获 panic）
```

---

### 可能的面试问题

**Q1：拦截器链用递归和用闭包迭代有什么区别？**

A：递归每一层都占用一个栈帧，如果拦截器数量很多（比如 100 个），会有 100 层调用栈，极端情况下可能栈溢出，且 pprof 看到的调用栈很深不好排查。闭包迭代（本项目的做法）是反向循环构建闭包链，运行时调用栈只有 1 层（最外层调用），内部通过函数指针跳转，栈深度固定。gRPC-go 也采用类似方案。

**Q2：Recovery 拦截器必须排第一吗？顺序错了会怎样？**

A：必须排第一。拦截器链是嵌套包裹的，最外层能捕获所有内层的 panic。如果 Recovery 排在 Logging 后面：`Logging.before → Recovery → handler → Logging.after`，handler 如果 panic 会被 Recovery 捕获没问题，但 Logging 的 `.after` 部分永远不会被执行（panic 展开不经过 defer 以外的代码）。Recovery 在最外层能确保无论哪一层 panic，都能被捕获并且 Logging 也能看到这次失败。

**Q3：Prometheus 的 Counter 和 Histogram 分别监控什么？**

A：Counter 只增不减，用于统计累计次数。`rpc_calls_total` 除以时间范围可得 QPS，按 `status=error` 过滤可得错误率。Histogram 记录分布区间，`rpc_duration_seconds` 可以查询 P50/P90/P99 延迟，即"99% 的请求在 X 毫秒内完成"。监控系统一般同时需要这两个，QPS + 错误率 + 延迟是黄金三角指标。

**Q4：令牌桶和漏桶（Leaky Bucket）有什么区别？**

A：漏桶以**固定速率**处理请求，多余的请求排队或丢弃，完全不允许突发。令牌桶以固定速率**补充令牌**，桶满时令牌积累，允许短暂消耗积累的令牌来处理突发流量。例如速率 100 req/s、桶容量 200：平静 2 秒后来了 200 个请求，令牌桶允许立即处理完，漏桶需要 2 秒才能处理完。前者适合 API 限速，后者适合严格控制输出速率（比如消息队列消费）。

**Q5：Retry 的固定间隔有什么问题？生产中怎么改进？**

A：固定间隔的问题是**惊群效应（Thundering Herd）**：大量请求在同一时刻失败，然后在同一时刻重试，对下游服务造成突发冲击，可能引发连锁雪崩。改进方案：**指数退避 + 抖动（Exponential Backoff + Jitter）**：`interval = min(cap, base * 2^attempt) + random(0, base)`。每次重试间隔翻倍，加上随机抖动打散重试时刻，AWS 的 SDK 都用这个策略。

**Q6：限流应该放在哪一层？服务端还是客户端？**

A：两层都需要，作用不同。服务端限流是**自我保护**，防止下游过载，不管流量来自谁都限。客户端限流是**源头控制**，在不必要的请求发出之前就拒绝，节省网络资源和服务端压力。生产中常见三层：网关层（全局 QPS）→ 客户端 SDK（per-service）→ 服务端（per-method），形成漏斗型防御。

---

## Bullet 4 — 熔断器 · 负载均衡

### 全流程

#### 熔断器三态状态机

```
                  失败率 >= 50% && 总请求 >= 5
Closed ──────────────────────────────────────────────► Open
  ▲                                                       │
  │                                                       │ 超过 60s
  │                                                       ▼
  └──── 连续成功 >= 2 次 ──────────────────────── HalfOpen
```

**三种状态的行为：**

| 状态 | 行为 |
|------|------|
| Closed（关闭）| 正常放行，记录成功/失败，达到阈值时转 Open |
| Open（打开）| 直接返回 `ErrCircuitOpen`，不发送任何请求；超过 Timeout 后转 HalfOpen |
| HalfOpen（半开）| 允许最多 `MaxRequests=1` 个探测请求；失败 → 回 Open；连续成功 `SuccessThreshold=2` 次 → 回 Closed |

**`beforeCall()` 的判断逻辑（加锁保护）：**

```go
switch state {
case Closed:   return nil              // 直接放行
case Open:
    if time.Since(openTime) > Timeout:
        state = HalfOpen               // 超时，允许探测
        return nil
    return ErrCircuitOpen              // 还在 Open 冷却期
case HalfOpen:
    if halfOpenInFlight >= MaxRequests:
        return ErrTooManyRequests      // 探测名额已满
    halfOpenInFlight++
    return nil
}
```

#### 滑动窗口（分桶实现）

```
窗口总时长 = Interval = 10s
桶数量 = 10
单桶时长 = 1s

时间轴：
[桶0][桶1][桶2][桶3][桶4][桶5][桶6][桶7][桶8][当前桶]
                                                  ↑ currentIndex（环形推进）
```

每次 `updateBuckets()`：

```
elapsed = now - lastUpdate
bucketsToAdvance = elapsed / bucketTime

if bucketsToAdvance >= size:    // 超过整个窗口时长，全部清零
    清空所有桶
else:
    循环推进 currentIndex，清零经过的桶（旧数据自动过期）
```

`FailureRate()` = 所有桶的 (failure + timeout) / total。失败率计算**包含 timeout**，超时的请求对系统同样有害。

#### 客户端熔断器的组织方式

```go
// Client 中：
breakers map[string]*circuitbreaker.CircuitBreaker  // key = service 名

getCircuitBreaker("UserService"):
  → 读锁查找
  → 不存在：写锁 → double-check → New(DefaultConfig()) → 存入 map
```

每个 service 独立一个 CircuitBreaker，UserService 的熔断不影响 OrderService。

#### 四种负载均衡算法

**1. 轮询（RoundRobin）：**

```go
idx := atomic.AddUint64(&rb.index, 1) % uint64(len(instances))
```

无锁原子操作，均匀分配，实现最简单。适合所有实例性能相近的场景。

**2. 随机（Random）：**

```go
idx := r.rnd.Intn(len(instances))
```

概率上均匀，代码极简。适合无状态服务、实例数量少的场景。

**3. 加权轮询（WeightedRoundRobin）：**

```
实例 A weight=200, 实例 B weight=100
→ weights 数组展开为：[A, A, B, A, A, B, ...]（按权重比 2:1 排列）
→ 按 current 指针循环
```

当 instances 列表变化时重新 `rebuild()`，保证权重比例正确。适合异构机器（高配机器给更高权重）。

**4. 一致性哈希（ConsistentHash）：**

```
每个实例生成 150 个虚拟节点：
  key = "{instance.ID}-{i}"，hash = MD5(key)[0:4]（取前 4 字节为 uint32）
  hashRing = 所有虚拟节点的 hash 值，排序后的 uint32 数组

Pick(key):
  hash = MD5(key)[0:4]
  idx = sort.Search(hashRing, hash >= target)  // 二分找第一个 >= hash 的位置
  return nodes[hashRing[idx]]                  // 顺时针找最近节点
```

虚拟节点的作用：如果每个实例只有 1 个真实节点，节点分布在哈希环上可能极不均匀（"堆在一起"）。150 个虚拟节点让分布更均匀，也让新增/删除节点时只影响 `1/N` 的 key 空间。

实例列表变化时调用 `rebuild()` 重建哈希环，`isSameInstances()` 用于判断是否需要重建（避免每次 Pick 都重建）。

---

### 可能的面试问题

**Q1：熔断器和限流有什么区别？**

A：限流是**预防性**的，限制请求的进入速率，保护服务不被过载，无论下游是否健康都生效。熔断是**响应性**的，当检测到下游已经不健康（高失败率）时，主动停止发送请求，给下游恢复时间，防止雪崩。两者互补：限流在入口，熔断在出口；限流针对 QPS，熔断针对错误率。

**Q2：HalfOpen 状态为什么只允许 1 个探测请求？**

A：下游服务刚从故障中恢复时，处理能力可能还没完全恢复（比如连接池重建中）。如果一次放入太多请求，可能立刻再次触发失败，反复在 Open 和 HalfOpen 之间抖动，延长恢复时间。只放 1 个探测请求，成功后才允许更多，这是保守但稳健的策略。

**Q3：为什么用分桶滑动窗口而不是精确滑动窗口？**

A：精确滑动窗口需要记录每一条请求的时间戳（如 `ratelimiter/sliding_window.go` 的实现），内存占用随 QPS 线性增长。分桶方案把时间切成固定的桶，只存每个桶的聚合计数，内存固定（`size` 个 Bucket），适合高 QPS 场景。代价是误差：时间窗口不是精确的"最近 10 秒"，而是"最近 10 个 1 秒桶"，最大误差 = 1 个桶的时长（1s）。对熔断器来说，1s 的误差完全可接受。

**Q4：一致性哈希和普通取模哈希有什么区别？为什么微服务要用一致性哈希？**

A：取模哈希：`key % N`。新增/删除一个节点时 N 变化，几乎所有 key 都会映射到不同节点，缓存全部失效，引发大规模穿透。一致性哈希：节点映射到哈希环，key 找顺时针最近节点。新增节点只影响它和前一个节点之间的 key（约 `1/N`），删除节点的 key 转移到它的后继节点，大多数 key 不受影响。对于**有状态服务**（用户会话、分布式缓存），相同 key 始终路由到同一节点，有局部性收益。

**Q5：150 个虚拟节点是怎么来的？太少或太多有什么影响？**

A：虚拟节点数没有理论最优值，150 是经验值（来自 Amazon Dynamo 论文的参考）。太少（比如 1）：节点在环上分布可能严重不均，某个节点承担 50% 的流量而其他节点只有 10%。太多（比如 10000）：内存和重建哈希环的时间开销增加，重建是 `O(N*V*logN)` 的排序操作。150 在均匀性和性能之间取得平衡，实际可根据实例数量和延迟要求调整。

**Q6：现在的熔断器是 per-service 粒度，有什么问题？怎么改进？**

A：per-service 的问题：一个 service 有 3 个实例，实例 C 故障率 100%，但 A 和 B 完全正常。per-service 熔断器会把整个 service 的请求统计进去，等失败率达到阈值时切断对整个 service 的访问，A 和 B 也会被误伤。改进：per-instance 粒度的熔断，每个 `ServiceInstance.ID` 独立维护一个 CircuitBreaker，对故障实例单独熔断并从负载均衡池中摘除，健康实例正常服务。

---

*文档更新时间：2026-03-27*
