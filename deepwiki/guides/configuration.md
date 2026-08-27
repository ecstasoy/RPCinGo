# 配置指南

## 概述

RPCinGo 支持两种配置方式：**代码（Options Pattern）** 和 **YAML 文件**。YAML 方式通过 `pkg/config` 解析后转换为 Options，适合生产部署；代码方式更灵活，适合测试和快速开发。

## 快速参考：完整 YAML 配置

```yaml
# configs/full.yaml
server:
  address: ":8080"
  codec: json              # json | protobuf
  compress: none           # none | gzip
  read_timeout: 30s
  write_timeout: 30s
  max_concurrent: 1000     # 0 = 无限制
  interceptors:
    - recovery             # 必须第一个
    - logging
    - metrics
    - rate_limit
  registry:
    type: etcd             # etcd | memory
    endpoints:
      - "localhost:2379"
    service_name: "my-service"
    address: "localhost:8080"
    lease_ttl: 10          # 租约 TTL（秒）

client:
  address: ""              # Fixed 模式填写；Discovery 模式留空
  codec: json
  load_balancer: round_robin  # round_robin | random | weighted_round_robin | consistent_hash
  timeout: 5s
  discovery:
    type: etcd
    endpoints:
      - "localhost:2379"
    service_name: "my-service"
  pool:
    min_size: 2
    max_size: 10
    idle_timeout: 5m
    max_lifetime: 30m
  circuit_breaker:
    enabled: true
    min_requests: 10
    failure_rate: 0.5      # 50% 失败率触发熔断
    recover_timeout: 10s
  rate_limiter:
    type: token_bucket     # token_bucket | sliding_window
    rate: 1000.0           # 每秒令牌数
    burst: 100             # 突发上限（仅令牌桶）
```

## 配置加载

```go
import "RPCinGo/pkg/config"

cfg, err := config.Load("configs/server.yaml")
if err != nil {
    log.Fatalf("load config: %v", err)
}

// 构建服务端 Options
serverOpts, err := cfg.BuildServerOptions()
srv := server.New(serverOpts...)

// 构建客户端 Options
clientOpts, err := cfg.BuildClientOptions()
cli := client.New(clientOpts...)
```

## 各组件配置详解

### 服务端（Server）

| 字段 | 类型 | 说明 | 推荐生产值 |
|------|------|------|-----------|
| `address` | string | 监听地址 | `:8080` 或 `0.0.0.0:8080` |
| `codec` | string | 编解码器 | `protobuf`（高性能） |
| `compress` | string | 压缩 | `gzip`（带宽受限场景） |
| `read_timeout` | duration | 读超时 | `30s` |
| `write_timeout` | duration | 写超时 | `30s` |
| `max_concurrent` | int | 最大并发请求 | `5000`（根据内存调整） |

### 连接池（Pool）

| 字段 | 类型 | 说明 | 推荐值 |
|------|------|------|--------|
| `min_size` | int | 最小连接数（预热） | `5` |
| `max_size` | int | 最大连接数 | QPS/100（经验值） |
| `idle_timeout` | duration | 空闲连接清理 | `3m` ~ `10m` |
| `max_lifetime` | duration | 连接强制刷新 | `30m` |

### 熔断器（Circuit Breaker）

| 字段 | 类型 | 说明 | 推荐值 |
|------|------|------|--------|
| `min_requests` | int | 触发判断的最小请求数 | `20`（避免冷启动误触发） |
| `failure_rate` | float | 失败率阈值 | `0.5`（50%） |
| `recover_timeout` | duration | 熔断后恢复探测等待 | `10s` ~ `30s` |

### 限流器（Rate Limiter）

| 场景 | 推荐类型 | 配置示例 |
|------|---------|---------|
| 普通 API 限流 | `token_bucket` | `rate: 1000, burst: 200` |
| 数据库写入保护 | `sliding_window` | `limit: 500, window: 1s` |
| 突发流量吸收 | `token_bucket` | `rate: 500, burst: 2000` |

## 环境特定配置

推荐为不同环境准备独立配置文件：

```
configs/
├── dev.yaml      # 开发：memory registry、低超时、详细日志
├── test.yaml     # 测试：memory registry、高限流
├── staging.yaml  # 预发：etcd、生产参数 1/10
└── prod.yaml     # 生产：etcd 集群、全功能、严格参数
```

**dev.yaml 示例**（无 etcd 依赖）：

```yaml
server:
  address: ":8080"
  codec: json
  compress: none
  max_concurrent: 100
  interceptors:
    - recovery
    - logging
  registry:
    type: memory  # 无需 etcd
```

## 常见配置错误

| 错误 | 症状 | 修复 |
|------|------|------|
| `max_concurrent: 0` 且流量大 | OOM 风险 | 设置合理上限，如 `5000` |
| `pool.min_size > pool.max_size` | panic 或连接耗尽 | 确保 min ≤ max |
| `failure_rate: 1.0` | 熔断从不触发 | 应使用 `0.5` 等合理值 |
| `interceptors` 中 `recovery` 不在首位 | 后续拦截器 panic 无法捕获 | 始终将 `recovery` 放第一位 |
| `codec: protobuf` + 服务未用 proto.Message | 运行时 encode 错误 | protobuf codec 要求参数实现 proto.Message |

## Source References

- `pkg/config/config.go`
- `configs/`
- `wiki/config/configuration.md`
