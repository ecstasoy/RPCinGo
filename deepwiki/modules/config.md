# 模块：Config（配置）

## 职责

- 定义 YAML 配置文件的结构体映射（`Config`、`ServerConfig`、`ClientConfig` 等）
- 提供 `Load(path)` 函数从文件加载配置
- 提供 `BuildServerOptions()` 和 `BuildClientOptions()` 将配置转换为 Options 列表
- 支持字符串枚举解析（Codec 名称、LoadBalancer 名称、Registry 类型）

## 关键文件

| 文件 | 职责 |
|------|------|
| `pkg/config/config.go` | 配置结构体定义、Load、Build 函数 |

## 配置结构层次

```yaml
# config.yaml 完整示例
server:
  address: ":8080"
  codec: json              # json / protobuf
  compress: none           # none / gzip
  read_timeout: 30s
  write_timeout: 30s
  max_concurrent: 1000
  interceptors:
    - recovery
    - logging
    - metrics
    - rate_limit
  registry:
    type: etcd             # etcd / memory
    endpoints: ["localhost:2379"]
    service_name: "my-service"
    address: "localhost:8080"
    lease_ttl: 10

client:
  address: ""              # Fixed 模式：填写此项
  codec: json
  load_balancer: round_robin  # round_robin / random / weighted_round_robin / consistent_hash
  discovery:
    type: etcd
    endpoints: ["localhost:2379"]
    service_name: "my-service"
  pool:
    min_size: 2
    max_size: 10
    idle_timeout: 5m
    max_lifetime: 30m
  circuit_breaker:
    enabled: true
    min_requests: 10
    failure_rate: 0.5
    recover_timeout: 10s
  rate_limiter:
    type: token_bucket     # token_bucket / sliding_window
    rate: 1000
    burst: 100
```

## Config 结构体

| 字段 | 类型 | 说明 |
|------|------|------|
| `Server` | `ServerConfig` | 服务端配置 |
| `Client` | `ClientConfig` | 客户端配置 |

### ServerConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Address` | `string` | `:8080` | 监听地址 |
| `Codec` | `string` | `json` | 编解码器类型 |
| `Compress` | `string` | `none` | 压缩类型 |
| `ReadTimeout` | `duration` | `30s` | 读超时 |
| `WriteTimeout` | `duration` | `30s` | 写超时 |
| `MaxConcurrent` | `int` | 0（无限） | 最大并发数 |
| `Interceptors` | `[]string` | `[]` | 启用的拦截器名称列表 |
| `Registry` | `RegistryConfig` | — | 注册中心配置 |

### ClientConfig

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Address` | `string` | — | Fixed 模式目标地址 |
| `Codec` | `string` | `json` | 编解码器类型 |
| `LoadBalancer` | `string` | `round_robin` | 负载均衡算法 |
| `Discovery` | `DiscoveryConfig` | — | 服务发现配置 |
| `Pool` | `PoolConfig` | — | 连接池配置 |
| `CircuitBreaker` | `CircuitBreakerConfig` | — | 熔断器配置 |
| `RateLimiter` | `RateLimiterConfig` | — | 限流器配置（客户端用） |

## 字符串枚举映射

| 配置值 | 映射结果 |
|--------|---------|
| `"json"` | `codec.NewJSONCodec()` |
| `"protobuf"` | `codec.NewProtobufCodec()` |
| `"gzip"` | `codec.NewGzipCompressor()` |
| `"round_robin"` | `loadbalancer.NewRoundRobin()` |
| `"random"` | `loadbalancer.NewRandom()` |
| `"weighted_round_robin"` | `loadbalancer.NewWeightedRoundRobin()` |
| `"consistent_hash"` | `loadbalancer.NewConsistentHash(150)` |
| `"etcd"` | `etcd.NewEtcdRegistry(endpoints)` |
| `"memory"` | `memory.NewMemoryRegistry()` |

## 输入与输出

| 类型 | 名称 | 位置 | 说明 |
|------|------|------|------|
| Input | YAML 文件路径 | `Load(path)` | `configs/*.yaml` |
| Output | `*Config` | `Load()` 返回 | 解析后的配置结构体 |
| Output | `[]server.Option` | `BuildServerOptions()` | 可直接传给 `server.New()` |
| Output | `[]client.Option` | `BuildClientOptions()` | 可直接传给 `client.New()` |

## 示例代码片段

**加载配置并启动服务端**：

```go
cfg, err := config.Load("configs/server.yaml")
if err != nil {
    log.Fatal(err)
}

opts, err := cfg.BuildServerOptions()
if err != nil {
    log.Fatal(err)
}

srv := server.New(opts...)
srv.RegisterService("Calculator", &CalculatorService{})
log.Fatal(srv.Start())
```

**加载配置并创建客户端**：

```go
cfg, err := config.Load("configs/client.yaml")
if err != nil {
    log.Fatal(err)
}

opts, err := cfg.BuildClientOptions()
if err != nil {
    log.Fatal(err)
}

cli := client.New(opts...)
defer cli.Close()
```

## 边界情况

- **未知 Codec 字符串**：`BuildServerOptions()` 返回 error，不 panic
- **YAML 文件不存在**：`Load()` 返回 `os.ErrNotExist`
- **部分字段缺失**：使用零值，调用方需注意（如 Address 为空字符串）
- **Duration 格式**：必须使用 Go 时间格式（如 `30s`、`5m`），YAML 库会自动解析

## 设计说明

Config 模块的核心价值在于**隔离配置格式与框架逻辑**：服务代码无需硬编码 `codec.NewJSONCodec()`，通过配置文件即可切换实现。`BuildXxxOptions()` 函数承担了字符串到对象的转换职责，类似于依赖注入容器的角色。

## Source References

- `pkg/config/config.go`
- `configs/`（配置文件示例）
- `wiki/config/configuration.md`
