# Bug 修复记录

> 修复日期：2026-03-25

---

## Bug 1 — Logging/Metrics 拦截器中 service/method 永远为空

### 问题

`pkg/interceptor/logging.go` 和 `pkg/interceptor/metrics.go` 都声明了局部变量但从未赋值：

```go
// 修复前（两个文件中均存在）
var service, method string
logger.Infof("→ RPC call: [%s.%s]", service, method)  // 永远输出 "[.]"
```

- 日志中所有调用记录均显示为 `[.]`，无法追踪请求来源。
- Prometheus 指标 `rpc_calls_total` 和 `rpc_duration_seconds` 中
  `service` / `method` label 全部为空字符串，图表和告警完全失效。

### 修复

```go
// 修复后
service, method := req.Service, req.Method
logger.Infof("→ RPC call: [%s.%s]", service, method)
```

**涉及文件：**
- `pkg/interceptor/logging.go:35`
- `pkg/interceptor/metrics.go:41`

---

## Bug 2 — 客户端无拦截器链，`WithRetry` 选项不生效

### 问题

服务端拥有完整的 `interceptor.Chain`，但客户端 `Call()` 直接调用底层传输，
没有任何中间件机制。`SendWithRetry` 虽然存在于 `tcp.Client`，但 `client.Client`
从未调用它。用户无法在客户端侧注入日志、追踪、重试等通用逻辑。

### 修复

#### 新增文件

**`pkg/interceptor/retry.go`** — 通用 Retry 拦截器

```go
interceptor.Retry(maxRetries int, interval time.Duration) Interceptor
```

- 对 **可重试错误**（网络/IO 错误、`Unavailable`、`DeadlineExceeded`、
  `ResourceExhausted`）进行最多 `maxRetries` 次重试，两次之间等待 `interval`。
- **不可重试错误**（`NotFound`、`InvalidArgument`、`PermissionDenied` 等应用级错误）
  立即返回，不浪费网络资源。
- 每次重试前检查 `ctx.Done()`，尊重调用方的超时/取消语义。

#### 修改文件

**`pkg/client/options.go`**

新增两个选项：

| 函数 | 说明 |
|------|------|
| `WithClientInterceptors(interceptors ...interceptor.Interceptor)` | 注册任意客户端拦截器 |
| `WithRetry(maxRetries int, retryInterval time.Duration)` | 启用自动重试，Retry 拦截器会被自动前置（最外层） |

**`pkg/client/client.go`**

1. `Client` 结构体增加 `interceptors []interceptor.Interceptor` 字段。
2. `NewClient` / `NewDiscoveryClient` 调用 `buildInterceptors(opts)` 初始化拦截器链；
   若 `maxRetries > 0`，自动在链首插入 `Retry` 拦截器。
3. `Call()` 重构：先构造 `*protocol.Request`，再通过 `interceptor.Chain.Intercept()`
   包裹实际 RPC 调用，最后断言返回 `*protocol.Response`。
4. `callFixed` / `callWithDiscovery` 签名从 `(ctx, service, method, args)` 改为
   `(ctx, *protocol.Request)`，消除重复的 `NewRequest` 构造。
5. 新增 `Use(interceptors ...interceptor.Interceptor)` 方法，支持在构造后追加拦截器。

#### 使用示例

```go
// 重试 2 次，间隔 200ms
cli, _ := client.NewClient("127.0.0.1:8080",
    client.WithRetry(2, 200*time.Millisecond),
    client.WithClientInterceptors(
        interceptor.Logging(nil),
    ),
)

// 或在构造后追加
cli.Use(interceptor.Metrics())
```

---

## Bug 3 — `pkg/config` 配置文件无法直接创建 Server / Client

### 问题

`pkg/config.Load()` 能解析 YAML，但没有工厂函数把配置结构体转换成
`server.Option` / `client.Option`。用户仍需手动将每个字段翻译成选项调用，
配置文件形同虚设。

### 修复

**`pkg/config/config.go`** 新增三组函数：

| 函数 | 说明 |
|------|------|
| `BuildServerOptions(cfg *Config) []server.Option` | 将 `ServerConfig` 转为选项切片 |
| `BuildClientOptions(cfg *Config) []client.Option` | 将 `ClientConfig`+`PoolConfig` 转为选项切片 |
| `parseCodecTypes(codec, compress string)` | 字符串 → `CodecType` / `CompressType` |
| `parseLoadBalancer(name string)` | 字符串 → `LoadBalancer` 实例 |

支持的负载均衡器字符串：`round_robin`/`rr`、`random`、`weighted`/`weighted_round_robin`、
`consistent_hash`/`consistent`。

> **注意**：Discovery 实例（etcd 连接等）需要外部依赖，工厂函数不自动创建，
> 调用方仍需通过 `client.WithDiscovery(...)` 传入。

#### 使用示例

```go
cfg, err := config.Load("config.yaml")
if err != nil { ... }

// Server
srv := server.NewServer(config.BuildServerOptions(cfg)...)

// Client（固定地址）
cli, err := client.NewClient(
    cfg.Client.Address,
    config.BuildClientOptions(cfg)...,
)

// Client（服务发现，追加 Discovery）
cli, err := client.NewDiscoveryClient(
    append(
        config.BuildClientOptions(cfg),
        client.WithDiscovery(etcdDiscovery),
    )...,
)
```

---

## 拦截器执行顺序说明

修复后客户端完整的拦截器执行顺序（以同时设置 Retry + Logging 为例）：

```
Retry.before
  └── Logging.before
        └── 实际 RPC (callFixed / callWithDiscovery)
      Logging.after  ← 记录每次尝试的耗时
Retry.after          ← 失败时重试整个内层链
```

这意味着 Logging 会记录每次重试的耗时，方便排查重试原因。
