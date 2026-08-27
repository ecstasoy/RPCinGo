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
