# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 深度笔记

`.claude/notes/` 下有一套权威的架构深挖笔记，改代码前先读一遍：

- `00-overview.md` —— 模块依赖图、完整请求生命周期（9 阶段）、贯穿全局的不变量、已知待整治清单
- `01-wire-layer.md` —— 协议/编解码/TCP（含 15 个坑）
- `02-client-pool.md` —— 客户端和连接池（含 12 个坑）
- `03-server-interceptor.md` —— 服务端、反射注册、拦截器链
- `04-discovery.md` —— 注册中心和负载均衡
- `05-resilience.md` —— 熔断器和限流器
- `06-observability.md` —— Logger / Tracing / Config

每份笔记都标了精确的 `file:line` 引用，碰到特定问题按索引跳。

## 常用命令

```bash
# 跑全量测试
go test ./...

# 跑指定包的测试
go test ./pkg/circuitbreaker/...

# 带 race detector
go test -race ./...

# 带覆盖率
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# 集成测试（需要 etcd —— 见下方依赖）
go test ./pkg/registry/etcd/...
go test ./test/e2e/...
go test ./test/integration/...
go test ./test/benchmark/...

# 重新生成 protobuf 代码
./scripts/gen-proto.sh
./scripts/gen-example-proto.sh

# 跑 calculator 示例
go run examples/calculator/server/main.go
go run examples/calculator/client/main.go
go run examples/calculator/bench/main.go -c 32 -d 30s

# 跑 microservice 示例（需要 etcd）
go run examples/microservice/services/user/main.go
go run examples/microservice/clients/user/main.go
```

## 测试依赖

etcd（registry/etcd 和 e2e 测试必需 —— 单元测试没有时会优雅跳过）：
```bash
docker run -d -p 2379:2379 --name etcd quay.io/coreos/etcd:v3.5.0 \
  etcd --advertise-client-urls=http://localhost:2379 \
       --listen-client-urls=http://0.0.0.0:2379
```

Jaeger（示例里的 tracing 用）：
```bash
docker run -d -p 14268:14268 -p 16686:16686 jaegertracing/all-in-one
```

## 架构

模块路径 `RPCinGo`，所有包都在 `pkg/` 下。

### 请求生命周期

```
cli.CallTyped() / cli.Call()
  → 客户端拦截器链（retry → ratelimit → tracing → logging）
  → 【discovery 模式】getInstances → LB.Pick → poolManager.GetConnection
    【fixed 模式】fixedPool.Get
  → 【discovery + 熔断开】cb.CallResponse 包着上面
  → tcp.Client.SendRequest【在这里分配 per-connection RequestID，覆写原值】
  → 线上：20 字节头 + body
  → tcp.Server 接收，按 RequestID 解复用，writer 协程串行写响应
  → 服务端拦截器链（ratelimit → recovery → logging → metrics → tracing）
  → 反射派发到注册的服务方法
  → 响应按 requestID 走 pendingRequests 路由回等待协程
```

### 协议层（`pkg/protocol`）

自定义二进制分帧：**20 字节定长头**（Magic 2B | Version 1B | MsgType 1B | Codec 1B | Compress 1B | Reserved 2B | RequestID 8B | BodyLen 4B，大端序）+ 变长 body。`RequestID` 是多路复用的 key —— TCP 客户端维护 `map[uint64]*pendingCall`，服务端可以乱序响应。

### 传输层（`pkg/transport/tcp`）

`tcp.Client` 在**一条持久连接**上发送，通过 `pendingRequests` 按 RequestID 解复用。`tcp.Server` 用两个 semaphore 限并发：一个给连接、一个给在飞请求。每连接有一个专门的 writer 协程从 `writeCh` 消费，把并发 handler 的响应串行化，防止帧交错。

### 连接池（`pkg/pool`）

按地址建的 channel 式连接池。discovery 模式下 `PoolManager` 按 instance 地址一池一池地管。池强制 min/max 大小、idle timeout、max lifetime，由后台 cleanup 协程回收。`ConnectionFactory` 是客户端注入的抽象。

### 客户端模式（`pkg/client`）

选项决定两种模式：
- **Fixed 模式**：单个池对应一个硬编码地址。
- **Discovery 模式**：`PoolManager` + 负载均衡 + **per-service** 熔断器；instance 列表靠 `Watcher` 保持新鲜。

### 拦截器链（`pkg/interceptor`）

洋葱模型 —— 先注册 = 最外层。客户端和服务端用同一个 `Interceptor` 类型和 `Chain()` 构造器，只是中心那个 `Invoker` 不同（客户端是真实 RPC 发送，服务端是方法派发）。

### 服务注册（`pkg/server`）

`RegisterService(impl)` 用反射扫所有导出方法。**实际接受 3 种签名**（见 `pkg/server/service.go:184-221`）：
- `func(any) (any, error)`
- `func(context.Context, any) (any, error)`
- `func(context.Context, *Req) (*Resp, error)` —— `*Req` 和 `*Resp` 都实现 `proto.Message`（推荐形式）

不匹配任何一种的方法**静默跳过**。方法索引为 `"ServiceName.MethodName"`。

### 注册中心 & 发现（`pkg/registry`）

`Registry`：Register / Deregister / Update / Heartbeat / Close。`Discovery`：GetInstances / Watch / Close。etcd 实现用 lease + KeepAlive（一个 `EtcdRegistry` 共用一条 lease），key 按服务名前缀。`memory.Registry` 用于测试。

### 负载均衡（`pkg/loadbalancer`）

4 种策略实现 `LoadBalancer.Pick(ctx, []ServiceInstance)`：`RoundRobin`（原子计数器）、`Random`、`Weighted`（权重展开的经典 WRR，非 Nginx smooth）、`ConsistentHash`（MD5 截前 4 字节哈希环，每实例 150 vNode）。`ConsistentHash` 实现了 `PickWithOptions` 支持按 key 亲和度，但 `PickOptions.key` **未导出** —— 外部调用者拿不到亲和度能力。

### 熔断器（`pkg/circuitbreaker`）

三态 FSM（Closed → Open → HalfOpen）。统计用 10 桶滑动时间窗。可配：`FailureThreshold`（**比率**，不是计数）、`MinRequests`（跳闸前最小请求数门闩 —— 低流量下永远不会跳）、`Timeout`（Open → HalfOpen）、`SuccessThreshold`（HalfOpen → Closed）。

### Codec & 压缩（`pkg/codec`）

全局注册表把 codec ID 映射到 `Codec` 实现（JSON、Protobuf；MsgPack 声明但未注册）。`CompressedCodec` 是装饰器，可包 Gzip —— Snappy 声明了但未实现。codec 和压缩类型写在协议头里让接收方知道怎么解，不需要协商。

### 可观测性（`pkg/logger`、`pkg/tracing`、`pkg/config`、`pkg/ratelimiter`）

- `pkg/logger`：slog 接口，按依赖注入分发，无单例。
- `pkg/tracing`：OTel + Jaeger collector exporter；propagator 是 W3C TraceContext + Baggage + B3 的复合。追踪上下文通过 `req.Metadata` 过线。
- `pkg/config`：YAML 加载 + `BuildServerOptions` / `BuildClientOptions`。注意：`Server/Client` 段用裸 `time.Duration`（ns 整数），`Pool` 段用字符串解析的 `Duration` 包装，不一致。
- `pkg/ratelimiter`：`RateLimiter` 接口（`Allow`/`AllowN`/`Wait`）+ 令牌桶 + 真滑动窗口两种实现。拦截器只调 `Allow`（非阻塞、快速失败）。

## 非显然的坑

后来人的踩坑时间：

- **`HandlerTimeout` 目前无 `server.Option` 封装**（API 缺口）。要设置得走 `transport.WithHandlerTimeout`，但 `server.NewServer` 不接受原生 `transport.ServerOption`。
- **`NewClient`（fixed 模式）静默忽略** `WithDiscovery` / `WithLoadBalancer` / `WithCircuitBreaker` —— 这些只在 `NewDiscoveryClient` 生效。Fixed 模式也永远不走熔断。
- **`PoolManager` 硬编码池大小 100/10**，`WithPoolSize` 在 discovery 模式下是 no-op。
- **`RequestID` 被赋值两次**：`protocol.NewRequest` 给全局原子 ID；`tcp.Client.SendRequest` 覆写成 per-connection 计数器 —— 后者才是 `pendingCall` 的 key。发送前打 `req.ID` 日志会误导。
- **Metrics collector 在 `init()` 里注册到 `prometheus.DefaultRegisterer`**，进程里两份本包副本会 panic。框架不暴露 `/metrics` 端点，示例见 `examples/calculator/server/main.go:85-88`。
- **客户端没有线上 cancel 帧**：`ctx` 取消只是本地摘掉 pending 条目，服务端 handler 跑完响应被丢弃。`HandlerTimeout` 是服务端唯一真正的预算。
- **etcd watcher 静默丢事件**：一个 `WatchResponse` 携带多个事件时只吐第一个（`pkg/registry/etcd/watcher.go:35-56`）。etcd lease 掉线后也**不重建** —— 注册方会静默失联。
- **服务端 `HandleRequest` 永远返回 `(*Response, nil)`**，错误经 `mapError` 序列化进响应里。transport 层的 "if handlerErr != nil" 分支是死代码。
