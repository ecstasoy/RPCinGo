# RPCinGo 架构文档

> 作者：Kunhua Huang | 语言：Go 1.24.5 | 模块名：RPCinGo

---

## 目录

1. [项目概览](#1-项目概览)
2. [目录结构](#2-目录结构)
3. [整体架构](#3-整体架构)
4. [协议层 pkg/protocol](#4-协议层-pkgprotocol)
5. [编解码层 pkg/codec](#5-编解码层-pkgcodec)
6. [传输层 pkg/transport](#6-传输层-pkgtransport)
7. [服务端 pkg/server](#7-服务端-pkgserver)
8. [客户端 pkg/client](#8-客户端-pkgclient)
9. [连接池 pkg/pool](#9-连接池-pkgpool)
10. [服务注册与发现 pkg/registry](#10-服务注册与发现-pkgregistry)
11. [负载均衡 pkg/loadbalancer](#11-负载均衡-pkgloadbalancer)
12. [拦截器 pkg/interceptor](#12-拦截器-pkginterceptor)
13. [限流器 pkg/ratelimiter](#13-限流器-pkgratelimiter)
14. [熔断器 pkg/circuitbreaker](#14-熔断器-pkgcircuitbreaker)
15. [配置 pkg/config](#15-配置-pkgconfig)
16. [mini-rpc（原型版本）](#16-mini-rpc原型版本)
17. [示例 examples](#17-示例-examples)
18. [数据流全链路](#18-数据流全链路)
19. [包依赖关系图](#19-包依赖关系图)

---

## 1. 项目概览

RPCinGo 是一个从零手写的 Go RPC 框架，目标是覆盖生产级 RPC 框架的核心能力：

| 能力 | 实现 |
|------|------|
| 自定义二进制协议 | 20 字节定长 Header + 变长 Body |
| 多种序列化 | JSON、Protobuf（MsgPack 预留） |
| 压缩支持 | Gzip、Snappy（预留） |
| TCP 长连接 | 连接复用、KeepAlive、NoDelay |
| 连接池 | 带健康检查、定时清理、最大/最小连接数 |
| 服务端反射注册 | 通过 `reflect` 自动扫描方法签名 |
| 拦截器链 | 类 gRPC Interceptor 模式 |
| 服务注册 | etcd v3 租约 + 心跳、内存注册表 |
| 服务发现 | Watch 机制 + 本地缓存 |
| 负载均衡 | 轮询、随机、加权轮询、一致性哈希 |
| 限流 | 令牌桶、滑动窗口 |
| 熔断 | 三态状态机（Closed / Open / Half-Open） |
| Metrics | Prometheus 集成 |
| Typed API | Protobuf 强类型调用 |

---

## 2. 目录结构

```
RPCinGo/
├── go.mod                        # 模块定义
├── mini-rpc/                     # 极简版 RPC（学习原型）
│   ├── client/client.go
│   ├── codec/codec.go, json.go
│   ├── protocol/message.go
│   ├── server/server.go
│   ├── transport/transport.go, tcp_client.go, tcp_server.go
│   └── examples/simple/
├── pkg/                          # 生产级 RPC 框架
│   ├── circuitbreaker/           # 熔断器
│   │   ├── breaker.go            # 主体逻辑 + 状态机
│   │   ├── state.go              # 状态枚举
│   │   └── window.go             # 滑动统计窗口
│   ├── client/                   # RPC 客户端
│   │   ├── client.go             # Client 核心
│   │   ├── error_map.go          # 协议错误 -> Go error 映射
│   │   └── options.go            # 客户端选项
│   ├── codec/                    # 序列化
│   │   ├── codec.go              # Codec 接口 + 注册表 + CompressedCodec
│   │   ├── compress.go           # Compressor 接口
│   │   ├── json.go               # JSON 实现
│   │   └── protobuf.go           # Protobuf 实现
│   ├── config/                   # 配置加载
│   │   └── config.go
│   ├── interceptor/              # 拦截器
│   │   ├── interceptor.go        # Interceptor 类型 + Chain
│   │   ├── logging.go            # 日志拦截器
│   │   ├── metrics.go            # Prometheus 指标拦截器
│   │   ├── ratelimit.go          # 限流拦截器
│   │   ├── recovery.go           # panic 恢复拦截器
│   │   └── retry.go              # 重试拦截器（可重试错误自动重试）
│   ├── loadbalancer/             # 负载均衡
│   │   ├── balancer.go           # LoadBalancer 接口
│   │   ├── roundrobin.go         # 轮询
│   │   ├── random.go             # 随机
│   │   ├── weighted.go           # 加权轮询
│   │   └── consistent.go         # 一致性哈希（虚拟节点）
│   ├── pool/                     # 连接池
│   │   ├── pool.go               # ConnectionPool + PooledConnection + Factory
│   │   └── pool_manager.go       # 多地址池管理
│   ├── protocol/                 # 协议定义
│   │   ├── header.go             # 20 字节 Header 结构与编解码
│   │   ├── request.go            # Request 结构
│   │   ├── response.go           # Response 结构
│   │   ├── error.go              # Error 结构 + 错误码
│   │   ├── metadata.go           # Metadata（K/V 键值对）
│   │   └── pb/protocol.pb.go     # Protobuf 生成文件（PayloadCodec 枚举）
│   ├── ratelimiter/              # 限流算法
│   │   ├── limiter.go            # RateLimiter 接口
│   │   ├── token_bucket.go       # 令牌桶
│   │   └── sliding_window.go     # 滑动窗口
│   ├── registry/                 # 服务注册/发现
│   │   ├── registry.go           # Registry / Discovery 接口
│   │   ├── instance.go           # ServiceInstance 结构
│   │   ├── watcher.go            # Watcher / Event 接口
│   │   ├── etcd/                 # etcd 实现
│   │   │   ├── etcd.go           # EtcdClient（连接管理）
│   │   │   ├── registry.go       # EtcdRegistry（注册）
│   │   │   ├── discovery.go      # EtcdDiscovery（发现）
│   │   │   └── watcher.go        # EtcdWatcher（Watch）
│   │   └── memory/               # 内存实现
│   │       └── memory.go
│   ├── server/                   # RPC 服务端
│   │   ├── server.go             # Server 主体
│   │   ├── service.go            # ServiceRegistry + 反射注册
│   │   ├── options.go            # 服务端选项
│   │   └── error_map.go          # Go error -> 协议错误映射
│   └── transport/                # 传输层
│       ├── transport.go          # 接口定义（ClientTransport / ServerTransport / Handler）
│       ├── options.go            # 传输层选项
│       └── tcp/
│           ├── server.go         # TCP 服务端
│           ├── client.go         # TCP 客户端
│           └── codec.go          # ProtocolCodec（Header + Body 的读写）
└── examples/
    ├── calculator/               # 计算器示例（Protobuf 强类型）
    └── microservice/             # 微服务示例（etcd 注册发现）
```

---

## 3. 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                         Client Side                         │
│                                                             │
│  app code                                                   │
│    │                                                        │
│    ▼                                                        │
│  pkg/client.Client.Call()                                   │
│    └── interceptor.Chain（客户端拦截器链）                   │
│          ├── Retry()      → 自动重试                        │
│          ├── Logging()    → 客户端日志                      │
│          └── invoker()                                      │
│                ├── fixedMode  → ConnectionPool → tcp.Client │
│                └── discoveryMode                            │
│                      ├── registry.Discovery (etcd/memory)   │
│                      ├── loadbalancer.LoadBalancer          │
│                      ├── circuitbreaker.CircuitBreaker      │
│                      └── pool.PoolManager → tcp.Client      │
│                                                             │
│  tcp.Client.Send()                                          │
│    └── ProtocolCodec.WriteRequest() ── TCP ──────────────►  │
└─────────────────────────────────────────────────────────────┘
                          ▼ TCP ▼
┌─────────────────────────────────────────────────────────────┐
│                         Server Side                         │
│                                                             │
│  tcp.Server.handleConnection()                              │
│    └── ProtocolCodec.ReadRequest()                          │
│          │                                                  │
│          ▼                                                  │
│  server.Server.HandleRequest()                              │
│    └── interceptor.Chain.Intercept()                        │
│          ├── Recovery() → panic 保护                        │
│          ├── Logging()  → 日志                              │
│          ├── Metrics()  → Prometheus                        │
│          ├── RateLimit() → 限流                             │
│          └── invoker()  → ServiceRegistry.GetHandler()      │
│                               └── 反射调用业务方法           │
│          ◄──────────────────────────────────────────────── │
│  ProtocolCodec.WriteResponse() ── TCP ──────────────────►   │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. 协议层 pkg/protocol

### 4.1 Header（20 字节定长）

```
Byte:  0  1  2  3  4  5  6  7  8  9  10 11 12 13 14 15 16 17 18 19
      ┌──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┬──┐
      │Magic │Ver│Typ│Cod│Cmp│Reserv │       RequestID       │BodyLen│
      └──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┴──┘
```

| 字段 | 字节 | 说明 |
|------|------|------|
| Magic | 0-1 | `0xCAFE`，魔数用于校验合法性 |
| Version | 2 | 协议版本，当前 `0x01` |
| MsgType | 3 | `0x01` Request / `0x02` Response |
| Codec | 4 | `0x00` JSON / `0x01` Protobuf / `0x02` MsgPack |
| Compress | 5 | `0x00` None / `0x01` Gzip / `0x02` Snappy |
| Reserved | 6-7 | 保留字段 |
| RequestID | 8-15 | uint64，原子自增，用于请求匹配 |
| BodyLength | 16-19 | uint32，Body 字节数 |

大端字节序（Big Endian）。

### 4.2 Request

```go
type Request struct {
    ID             uint64       // 自增 ID，由 atomic.AddUint64 生成
    Service        string       // 目标服务名
    Method         string       // 目标方法名
    ServiceVersion string       // 可选版本号
    Args           interface{}  // 参数（序列化后为 []byte 或 map）
    Timeout        int64        // 超时毫秒数
    IsStream       bool         // 是否流式（预留）
    Metadata       Metadata     // K/V 元数据
    CreatedAt      int64        // 创建时间 UnixNano
    ArgsCodec      PayloadCodec // 参数序列化格式
}
```

### 4.3 Response

```go
type Response struct {
    ID         uint64       // 对应 Request.ID
    Data       interface{}  // 响应数据
    Error      *Error       // 错误信息，nil 表示成功
    Metadata   Metadata
    ServerTime int64        // 服务端处理时间 UnixNano
    DataCodec  PayloadCodec
}
```

### 4.4 错误码

| 常量 | 值 | 语义 |
|------|----|------|
| ErrorCodeOK | 0 | 成功 |
| ErrorCodeCanceled | 1 | 已取消 |
| ErrorCodeUnknown | 2 | 未知错误 |
| ErrorCodeInvalidArgument | 3 | 参数非法 |
| ErrorCodeDeadlineExceeded | 4 | 超时 |
| ErrorCodeNotFound | 5 | 服务/方法不存在 |
| ErrorCodeAlreadyExists | 6 | 重复注册 |
| ErrorCodePermissionDenied | 7 | 权限拒绝 |
| ErrorCodeResourceExhausted | 8 | 资源耗尽 |
| ErrorRequestEntityTooLarge | 9 | 请求体过大 |
| ErrorCodeInternal | 13 | 内部错误 |
| ErrorCodeUnavailable | 14 | 服务不可用 |

错误码设计参考 gRPC Status Codes，便于跨系统映射。

### 4.5 Metadata

`Metadata` 是 `map[string]string` 的封装，提供线程安全的 `Get/Set/Clone`，并定义了标准 Key 常量：

```
MetaKeyTraceID  = "trace-id"
MetaKeyUserID   = "user-id"
MetaKeyAuth     = "authorization"
MetaKeyClientIP = "client-ip"
```

---

## 5. 编解码层 pkg/codec

### 5.1 接口

```go
type Codec interface {
    Encode(v interface{}) ([]byte, error)
    Decode(data []byte, v interface{}) error
    Name() string
}
```

### 5.2 注册表

使用全局 `sync.RWMutex` 保护的 map，通过 `Register(typ, codec)` 注册，`Get(typ)` 查找，`GetOrDefault(typ)` 找不到时回退 JSON。

### 5.3 实现

| 实现 | 类型常量 | 说明 |
|------|---------|------|
| `jsonCodec` | `CodecTypeJSON` | `encoding/json` |
| `protobufCodec` | `CodecTypeProtobuf` | `google.golang.org/protobuf/proto` |

### 5.4 压缩装饰器

`CompressedCodec` 包装任意 `Codec` 加上 `Compressor`（Gzip/Snappy），Encode 时先序列化后压缩，Decode 时先解压后反序列化。

```go
codec := NewCompressedCodec(jsonCodec, GzipCompressor{})
```

---

## 6. 传输层 pkg/transport

### 6.1 接口定义（transport.go）

```go
// 客户端侧
type ClientTransport interface {
    Dial(ctx context.Context, addr string) error
    Send(ctx context.Context, data []byte) ([]byte, error)
    Close() error
    IsConnected() bool
    LocalAddr() net.Addr
    RemoteAddr() net.Addr
}

// 服务端侧
type ServerTransport interface {
    Listen(ctx context.Context, addr string) error
    Serve(ctx context.Context, handler Handler) error
    Close() error
    Addr() net.Addr
}

// 请求处理器（服务端回调）
type Handler func(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
```

### 6.2 TCP 实现

#### tcp/codec.go — ProtocolCodec

负责在 `net.Conn` 上的帧级读写：

```
WriteRequest:
  1. codec.Encode(req) -> bodyBytes
  2. NewHeader(..., len(bodyBytes))
  3. conn.Write(header.Encode() + bodyBytes)

ReadRequest:
  1. io.ReadFull(conn, [20]byte) -> header.Decode()
  2. io.ReadFull(conn, [header.BodyLength]byte) -> codec.Decode(req)
```

#### tcp/server.go — tcp.Server

- `Listen()` 调用 `net.ListenConfig.Listen()` 创建 TCP listener
- `Serve()` 启动 accept 循环，每个连接 goroutine 化
- 连接级并发控制：`connSemaphore`（channel 信号量）
- 请求级并发控制：`reqSemaphore`
- 支持 KeepAlive、NoDelay、ReadBuffer、WriteBuffer 配置
- 原子计数 `activeConnections` / `totalConnections` 用于监控

#### tcp/client.go — tcp.Client

- 持有单条 `net.TCPConn`
- `Send()` = 写请求帧 + 读响应帧（同步，连接级串行）
- `SendRequest()` 封装 `Send()`，处理 Request -> bytes -> Response

---

## 7. 服务端 pkg/server

### 7.1 Server 结构

```go
type Server struct {
    opts         *serverOptions
    registry     *ServiceRegistry    // 本地方法注册表
    Transport    *tcp.Server
    codec        codec.Codec
    serviceInstance *registry.ServiceInstance
    interceptors []interceptor.Interceptor
}
```

### 7.2 启动流程

```
NewServer(opts...)
    │
    ▼
Server.Start(ctx)
    ├── Transport.Listen(addr)        // 绑定端口
    ├── registerService()             // 注册到 etcd（可选）
    ├── go startHeartbeat()           // 定时心跳（可选）
    └── Transport.Serve(ctx, handler) // 进入 accept 循环
```

### 7.3 请求处理链

```
HandleRequest(ctx, req)
    └── interceptor.Chain.Intercept(ctx, req, invoker)
              ├── Recovery()
              ├── Logging()
              ├── Metrics()
              ├── RateLimit()
              └── invoker(ctx, req)
                    └── ServiceRegistry.GetHandler(service, method)
                              └── MethodHandler(ctx, req)  // 反射调用
```

### 7.4 服务反射注册

`RegisterService(name, impl)` 使用 `reflect` 遍历 `impl` 的公开方法，识别三种签名：

| Kind | 方法签名 | 说明 |
|------|---------|------|
| `rpcMethodArgsOnly` | `(args interface{}) (interface{}, error)` | 无 ctx |
| `rpcMethodCtxArgs` | `(ctx, args interface{}) (interface{}, error)` | 有 ctx，弱类型 |
| `rpcMethodTyped` | `(ctx, *ProtoMsg) (*ProtoMsg, error)` | 有 ctx，强类型 Protobuf |

强类型方法会自动处理 Protobuf / JSON 参数的反序列化，对调用方透明。

### 7.5 拦截器注册

```go
srv.Use(
    interceptor.Recovery(),
    interceptor.Logging(nil),
    interceptor.Metrics(),
)
```

按注册顺序执行（链式包裹，最先注册的最外层）。

---

## 8. 客户端 pkg/client

### 8.1 两种工作模式

#### 固定地址模式（Direct）

```go
cli, _ := client.NewClient("127.0.0.1:8080", opts...)
```

内部持有单个 `ConnectionPool`，适合点对点调用或测试。

#### 服务发现模式（Discovery）

```go
cli, _ := client.NewDiscoveryClient(
    client.WithDiscovery(etcdDiscovery),
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
    client.WithCircuitBreaker(true),
)
```

内部使用 `PoolManager` 管理多地址连接池，调用时动态从注册中心获取实例。

### 8.2 调用路径

```
Client.Call(ctx, service, method, args)
    │
    ├── protocol.NewRequest(service, method, args)   // 构造 Request
    │
    └── interceptor.Chain.Intercept(ctx, req, invoker)
              ├── Retry()           // 可选，outermost
              ├── Logging()         // 可选，用户注册
              └── invoker(ctx, req)
                    ├── fixedMode  → callFixed(ctx, req)
                    │       └── pool.Get() → conn.SendRequest()
                    │
                    └── discoveryMode
                            ├── circuitbreaker.CallResponse()  // 可选熔断
                            └── callWithDiscovery(ctx, req)
                                    ├── getInstances()          // 本地缓存 + etcd
                                    ├── loadBalancer.Pick()     // 选实例
                                    ├── poolManager.GetConnection(endpoint)
                                    └── conn.SendRequest()
```

### 8.3 服务实例缓存与 Watch

首次 `getInstances()` 从 etcd 拉取后缓存到 `instanceCache`。若开启 `enableWatch`，后台启动 goroutine 监听 etcd 变更事件，自动更新缓存（增/删/改）。

### 8.4 客户端拦截器链

客户端拥有与服务端对称的拦截器机制，复用 `interceptor.Interceptor` 类型。

**构造时注册（推荐）：**

```go
cli, _ := client.NewClient("127.0.0.1:8080",
    client.WithRetry(2, 200*time.Millisecond),          // 失败最多重试 2 次
    client.WithClientInterceptors(interceptor.Logging(nil)),
)
```

`WithRetry` 会将 `Retry` 拦截器自动前置（outermost），确保重试包裹所有内层拦截器。

**构造后追加：**

```go
cli.Use(interceptor.Metrics())
```

**拦截器执行顺序（Retry + Logging 示例）：**

```
Retry.before
  └── Logging.before
        └── 实际 RPC（callFixed / callWithDiscovery）
      Logging.after  ← 记录每次尝试耗时
Retry.after          ← 失败时重试整个内层链
```

### 8.5 强类型调用

```go
func (c *Client) CallTyped(ctx context.Context, service, method string, req proto.Message, resp proto.Message) error
```

将 `Call()` 的 `*protocol.Response` 中的 `Data` 反序列化到 `resp`，支持 Protobuf 和 JSON 两种 payload codec。

---

## 9. 连接池 pkg/pool

### 9.1 ConnectionPool

基于 `chan *PooledConnection` 实现无锁快路径：

```
Get():
  select {
    case conn := <-pool: // 有空闲连接
      校验健康 + 过期 → return
    default:             // 无空闲，新建
      createNewConnection()
  }

Put(conn):
  if 健康 && 未过期:
    select {
      case pool <- conn: // 放回
      default: conn.Close() // 池满丢弃
    }
```

后台两个 goroutine：
- `cleanupRoutine`：按 `CleanupInterval` 清理超时/过期连接，并补充到 `MinSize`
- `healthCheckRoutine`：按 `HealthCheckInterval` 检查连接可用性

### 9.2 默认参数

| 参数 | 默认值 |
|------|--------|
| MaxSize | 100 |
| MinSize | 10 |
| MaxIdleTime | 90s |
| MaxLifetime | 30min |
| CleanupInterval | 30s |
| HealthCheckInterval | 60s |
| DialTimeout | 5s |
| WaitTimeout | 5s |

### 9.3 ConnectionFactory

接口 `Create(address) (*tcp.Client, error)` 解耦连接创建逻辑：

- `DefaultConnectionFactory`：标准 TCP 连接
- `RetryConnectionFactory`：带重试的工厂（装饰器模式）
- `MockConnectionFactory`：测试用 mock

### 9.4 PoolManager（pool_manager.go）

管理多地址的 `ConnectionPool`，以 `address` 为 key，按需创建池，`RemovePool(addr)` 在服务实例下线时释放。

---

## 10. 服务注册与发现 pkg/registry

### 10.1 接口

```go
type Registry interface {
    Register(ctx, instance) error
    Deregister(ctx, service, instanceID) error
    Update(ctx, instance) error
    Heartbeat(ctx, service, instanceID) error
    Close() error
}

type Discovery interface {
    GetInstances(ctx, service) ([]*ServiceInstance, error)
    Watch(ctx, service) (Watcher, error)
    Close() error
}
```

`RegistryDiscovery` 嵌入两者，一个对象同时具备注册和发现能力。

### 10.2 ServiceInstance

```go
type ServiceInstance struct {
    ID       string            // "{service}-{host}:{port}-{unix}"
    Service  string
    Version  string
    Address  string
    Port     int
    Metadata map[string]string
    Weight   int               // 默认 100，用于加权负载均衡
    Status   InstanceStatus    // Up/Down/Starting/Unknown
}
```

### 10.3 etcd 实现

**EtcdRegistry（注册）：**
1. 创建 etcd 租约（TTL = `LeaseTTL` 秒，默认 10s）
2. 启动 `KeepAlive` goroutine 维持租约
3. `Register()` = `Put(key, json(instance), withLease)`
4. `Deregister()` = `Delete(key)`
5. `Heartbeat()` = `KeepAliveOnce(leaseID)`
6. `Close()` = 撤销租约 + 关闭客户端

Key 格式：`/rpc/services/{service}/{instanceID}`

**EtcdDiscovery（发现）：**
- `GetInstances()` = `Get(prefix)` 扫描前缀，JSON 反序列化
- `Watch()` = etcd Watch API，返回 `EtcdWatcher`

### 10.4 内存实现（memory）

线程安全的 map，用于单元测试和本地开发。

---

## 11. 负载均衡 pkg/loadbalancer

### 11.1 接口

```go
type LoadBalancer interface {
    Pick(ctx context.Context, instances []*ServiceInstance) (*ServiceInstance, error)
    Name() string
}
```

### 11.2 实现对比

| 算法 | 结构 | 特点 |
|------|------|------|
| `RoundRobinBalancer` | `uint64` 原子计数 | 均匀轮询，无锁 |
| `RandomBalancer` | `math/rand` | 随机选择 |
| `WeightedRoundRobin` | 当前权重数组 | 平滑加权轮询（Smooth Weighted） |
| `ConsistentHash` | 虚拟节点哈希环（MD5，150 节点/实例） | 相同 key 映射到相同实例，适合有状态服务 |

一致性哈希在实例列表变化时自动重建哈希环。

---

## 12. 拦截器 pkg/interceptor

### 12.1 类型定义

```go
type Invoker     func(ctx context.Context, req *protocol.Request) (any, error)
type Interceptor func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error)
```

### 12.2 Chain 执行顺序

`NewChain(A, B, C)` 构建后执行顺序：

```
A.before → B.before → C.before → 真实 invoker → C.after → B.after → A.after
```

使用反向迭代 + 闭包实现，不依赖递归，避免栈溢出。

### 12.3 内置拦截器

| 拦截器 | 适用侧 | 功能 |
|--------|--------|------|
| `Recovery()` | 服务端 | `defer/recover` 捕获 panic，打印调用栈，转为 error 返回 |
| `Logging(logger)` | 双侧 | 记录调用方法、耗时、成功/失败，支持自定义 Logger |
| `Metrics()` | 双侧 | Prometheus Counter/Histogram，记录请求数和延迟 |
| `RateLimit(limiter)` | 服务端 | 调用 `RateLimiter.Allow()`，超限返回 `ErrorCodeResourceExhausted` |
| `Retry(maxRetries, interval)` | 客户端 | 对可重试错误（网络/IO、`Unavailable`、`DeadlineExceeded`、`ResourceExhausted`）自动重试；应用级错误（`NotFound`、`InvalidArgument` 等）立即返回 |

熔断器也可作为拦截器使用：`circuitbreaker.CircuitBreakerInterceptor(cb)`。

---

## 13. 限流器 pkg/ratelimiter

### 13.1 接口

```go
type RateLimiter interface {
    Allow(ctx context.Context) bool       // 非阻塞，判断是否允许
    AllowN(ctx context.Context, n int) bool
    Wait(ctx context.Context) error       // 阻塞等待直到允许或 ctx 超时
    Name() string
}
```

### 13.2 令牌桶（TokenBucketLimiter）

- 参数：`rate`（每秒令牌数）、`capacity`（桶容量）
- `refill()` 使用纳秒级精度补充令牌，保存 `nsRemainder` 避免精度损失
- `Wait()` 计算需要等待的时间，用 `time.NewTimer` 实现精确等待

### 13.3 滑动窗口（SlidingWindowLimiter）

- 参数：`limit`（窗口内最大请求数）、`window`（窗口时长）
- 每次 `Allow()` 清理过期时间戳，统计窗口内请求数
- 内存占用随 QPS 线性增长，适合中低流量场景

---

## 14. 熔断器 pkg/circuitbreaker

### 14.1 三态状态机

```
Closed ──(失败率 >= threshold)──► Open
  ▲                                 │
  │                                 │ (超过 Timeout)
  │                                 ▼
  └──(连续成功 >= threshold)── HalfOpen
```

### 14.2 配置（DefaultConfig）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| MaxRequests | 1 | Half-Open 状态下允许的最大并发探测请求数 |
| MinRequests | 5 | Closed 状态下触发评估的最小请求数 |
| Interval | 10s | 统计窗口时长 |
| Timeout | 60s | Open 状态持续时间，之后转 Half-Open |
| FailureThreshold | 0.5 | 失败率阈值，超过则打开熔断器 |
| SuccessThreshold | 2 | Half-Open 状态下连续成功次数，恢复 Closed |

### 14.3 滑动统计窗口（SlidingWindow）

分桶（Bucket）设计，`window.Interval / 10` 为单桶时长（默认 1s/桶，10 桶）。
`RecordSuccess/RecordFailure` 写当前桶，`FailureRate()` 汇总所有桶计算失败率，过期桶自动清零。

### 14.4 使用方式

**服务端拦截器：**
```go
cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
srv.Use(circuitbreaker.CircuitBreakerInterceptor(cb))
```

**客户端侧（内置）：**
```go
cli, _ := client.NewDiscoveryClient(
    client.WithCircuitBreaker(true), // 每个 service 独立一个熔断器
)
```

---

## 15. 配置 pkg/config

### 15.1 YAML 加载

`config.Load(path)` 将 YAML 文件反序列化到 `Config` 结构体，包含四个顶层块：

| YAML 块 | Go 结构体 | 说明 |
|---------|----------|------|
| `server` | `ServerConfig` | 地址、codec、超时、并发度、注册中心 |
| `client` | `ClientConfig` | 模式、连接数、超时、负载均衡、熔断 |
| `pool` | `PoolConfig` | 连接池详细参数（可与 client 块互补） |
| `registry` | `RegistryConfig` | 注册中心类型（etcd/memory）及 etcd 参数 |

### 15.2 工厂函数

读取配置后，通过工厂函数一步生成选项切片，消除手动字段翻译：

```go
cfg, _ := config.Load("config.yaml")

// Server
srv := server.NewServer(config.BuildServerOptions(cfg)...)

// Client（固定地址）
cli, _ := client.NewClient(cfg.Client.Address, config.BuildClientOptions(cfg)...)

// Client（服务发现，追加 Discovery）
cli, _ := client.NewDiscoveryClient(
    append(config.BuildClientOptions(cfg), client.WithDiscovery(myDiscovery))...,
)
```

额外选项可直接 append 覆盖工厂默认值。

**`BuildServerOptions`** 覆盖范围：地址、Codec/Compress、读写超时、并发度、心跳间隔。

**`BuildClientOptions`** 覆盖范围：Codec/Compress、连接池大小（取 `client` 与 `pool` 块的较大值）、调用超时、负载均衡器、Watch、熔断器开关。

**负载均衡器字符串映射：**

| YAML 值 | 实现 |
|---------|------|
| `round_robin` / `rr` | `RoundRobinBalancer` |
| `random` | `Random` |
| `weighted` / `weighted_round_robin` | `WeightedRoundRobin` |
| `consistent_hash` / `consistent` | `ConsistentHash` |

> Discovery 实例（需要 etcd 连接）不由工厂创建，仍需调用方通过 `client.WithDiscovery(...)` 传入。

---

## 16. mini-rpc（原型版本）

`mini-rpc/` 是框架的极简原型，用于理解 RPC 核心机制：

| 组件 | 说明 |
|------|------|
| `protocol/message.go` | 简单消息结构（无 Header，JSON 编解码） |
| `codec/codec.go` | Codec 接口 + JSON 实现 |
| `transport/` | TCP Server/Client，无连接池，无拦截器 |
| `server/server.go` | 极简服务端，map 存储方法处理器 |
| `client/client.go` | 极简客户端，每次调用新建连接 |

`mini-rpc` 与 `pkg/` 相互独立，不共享代码，适合学习对比。

---

## 17. 示例 examples

### 17.1 Calculator（计算器）

演示 Protobuf 强类型 RPC 调用：

```go
// Server
srv.RegisterService("Calculator", &CalculatorService{})

// Client
addReq := &calculator.AddRequest{A: 10, B: 20}
addResp := &calculator.AddResponse{}
cli.CallTyped(ctx, "Calculator", "Add", addReq, addResp)
```

### 17.2 Microservice（微服务）

演示 etcd 服务注册与发现：

```go
// Service
srv := server.NewServer(
    server.WithRegistry(etcdRegistry),
    server.WithServiceName("UserService"),
)

// Client
cli := client.NewDiscoveryClient(
    client.WithDiscovery(etcdDiscovery),
    client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
)
```

---

## 18. 数据流全链路

以 `Calculator.Add(10, 20)` 为例：

```
1. Client.Call("Calculator", "Add", addReq)
   └── protocol.NewRequest("Calculator", "Add", addReq)  // ID=1 原子自增

2. 客户端拦截器链（如已注册）
   Retry.before → Logging.before → invoker()

3. tcp.Client.Send()
   ├── codec.Encode(req) → JSON bytes
   ├── Header{Magic:0xCAFE, Type:Request, Codec:JSON, RequestID:1, BodyLen:N}
   └── conn.Write(header[20] + body[N])

4. TCP 传输

5. tcp.Server.handleConnection()
   ├── io.ReadFull(conn, 20) → Header.Decode()
   ├── io.ReadFull(conn, N)  → codec.Decode(Request)
   └── handler(ctx, req)

6. 服务端拦截器链
   Recovery → Logging → Metrics → invoker()

7. ServiceRegistry.GetHandler("Calculator", "Add")
   └── reflect 调用 CalculatorService.Add(ctx, *AddRequest) (*AddResponse, error)

8. 构造 Response{ID:1, Data:AddResponse{Result:30}}

9. tcp.Server
   ├── codec.Encode(resp) → JSON bytes
   ├── Header{Type:Response, RequestID:1}
   └── conn.Write(header + body)

10. tcp.Client.Send() 读取响应帧，返回 body bytes

11. 客户端拦截器链（返回路径）
    Logging.after（记录耗时）→ Retry.after（无错误，不重试）

12. Client.CallTyped() json.Unmarshal(data, addResp)
```

---

## 19. 包依赖关系图

```
examples/calculator
    └── pkg/server, pkg/client, pkg/protocol

pkg/server
    ├── pkg/protocol        (Request/Response/Error)
    ├── pkg/codec           (序列化)
    ├── pkg/transport/tcp   (网络传输)
    ├── pkg/registry        (注册接口)
    └── pkg/interceptor     (拦截器链)

pkg/client
    ├── pkg/protocol
    ├── pkg/codec
    ├── pkg/pool            (连接池)
    ├── pkg/registry        (发现接口)
    ├── pkg/loadbalancer    (负载均衡)
    ├── pkg/circuitbreaker  (熔断器)
    └── pkg/interceptor     (客户端拦截器链)

pkg/pool
    └── pkg/transport/tcp   (tcp.Client)

pkg/transport/tcp
    └── pkg/protocol        (Header/Codec类型)
    └── pkg/transport       (接口定义)

pkg/interceptor
    └── pkg/protocol        (Request类型)

pkg/circuitbreaker
    ├── pkg/interceptor     (作为拦截器)
    └── pkg/protocol

pkg/loadbalancer
    └── pkg/registry        (ServiceInstance)

pkg/registry/etcd
    └── pkg/registry        (接口)

pkg/registry/memory
    └── pkg/registry        (接口)

pkg/config
    ├── pkg/server          (BuildServerOptions)
    ├── pkg/client          (BuildClientOptions)
    ├── pkg/loadbalancer    (parseLoadBalancer)
    └── pkg/protocol        (CodecType/CompressType枚举)

pkg/ratelimiter             (无内部依赖)
pkg/protocol                (无内部依赖，叶节点)
pkg/codec
    └── pkg/protocol        (CodecType枚举)
```

---

*文档生成时间：2026-03-25 | 最后更新：2026-03-25（客户端拦截器链、Retry 拦截器、Config 工厂函数）*
