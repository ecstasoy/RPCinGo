# 配置管理

## 概述

`pkg/config` 提供基于 YAML 文件的配置加载，并提供 `BuildServerOptions` 和 `BuildClientOptions` 两个 Builder 函数，将配置结构体转换为 Server/Client 的 Options，实现配置文件与代码解耦。

**源码位置**：`pkg/config/config.go`（223 行）

**依赖**：`gopkg.in/yaml.v3 v3.0.1`

## 完整配置文件结构

```yaml
# config.yaml
server:
  address: "127.0.0.1:8080"    # 监听地址
  codec: "json"                 # json | protobuf
  compress: "none"              # none | gzip
  read_timeout: "5s"            # 读超时（Go time.Duration 格式）
  write_timeout: "5s"           # 写超时
  max_concurrent: 1000          # 最大并发请求数
  worker_pool_size: 100         # goroutine 池大小
  enable_registry: true         # 是否启用服务注册
  service_name: "UserService"   # 注册的服务名
  service_version: "1.0.0"      # 服务版本
  heartbeat_interval: "10s"     # 心跳间隔

client:
  mode: "discovery"             # fixed | discovery
  address: "127.0.0.1:8080"    # fixed 模式的服务地址
  codec: "protobuf"
  compress: "none"
  call_timeout: "5s"            # 单次 RPC 调用超时
  max_connections: 50           # 每地址连接池上限
  min_connections: 5            # 每地址连接池下限
  idle_timeout: "90s"           # 连接空闲超时
  load_balancer: "round_robin"  # round_robin | random | weighted | consistent
  circuit_breaker: true         # 是否启用熔断
  watch: true                   # Discovery 模式：是否启用后台 Watch

pool:
  min_size: 5
  max_size: 50
  idle_timeout: "90s"
  max_lifetime: "30m"
  health_check_interval: "30s"
  dial_timeout: "5s"
  read_timeout: "30s"
  write_timeout: "30s"

registry:
  type: "etcd"                  # etcd | memory
  etcd:
    endpoints:
      - "localhost:2379"
      - "localhost:2380"        # 多节点高可用
    dial_timeout: "5s"
    key_prefix: "/rpc/services"
    lease_ttl: 30               # 租约 TTL（秒）

circuit_breaker:
  max_requests: 3               # Half-Open 最多放行的探测请求数
  min_requests: 10              # 触发熔断最少请求样本
  interval: "30s"               # Closed 状态统计窗口
  timeout: "10s"                # Open 状态持续时间
  failure_threshold: 0.5        # 失败率阈值
  success_threshold: 2          # Half-Open 恢复需要的连续成功次数

rate_limiter:
  type: "token_bucket"          # token_bucket | sliding_window
  rate: 10000                   # 令牌桶：每秒令牌数 / 滑动窗口：每窗口最大请求数
  burst: 500                    # 令牌桶突发容量
  window: "1s"                  # 滑动窗口大小
```

## Go 结构体定义

```go
// pkg/config/config.go
type Config struct {
    Server         ServerConfig         `yaml:"server"`
    Client         ClientConfig         `yaml:"client"`
    Pool           PoolConfig           `yaml:"pool"`
    Registry       RegistryConfig       `yaml:"registry"`
    CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
    RateLimiter    RateLimiterConfig    `yaml:"rate_limiter"`
}

type ServerConfig struct {
    Address           string `yaml:"address"`
    Codec             string `yaml:"codec"`
    Compress          string `yaml:"compress"`
    ReadTimeout       string `yaml:"read_timeout"`
    WriteTimeout      string `yaml:"write_timeout"`
    MaxConcurrent     int    `yaml:"max_concurrent"`
    WorkerPoolSize    int    `yaml:"worker_pool_size"`
    EnableRegistry    bool   `yaml:"enable_registry"`
    ServiceName       string `yaml:"service_name"`
    ServiceVersion    string `yaml:"service_version"`
    HeartbeatInterval string `yaml:"heartbeat_interval"`
}

type ClientConfig struct {
    Mode           string `yaml:"mode"`             // "fixed" | "discovery"
    Address        string `yaml:"address"`
    Codec          string `yaml:"codec"`
    Compress       string `yaml:"compress"`
    CallTimeout    string `yaml:"call_timeout"`
    MaxConnections int    `yaml:"max_connections"`
    MinConnections int    `yaml:"min_connections"`
    IdleTimeout    string `yaml:"idle_timeout"`
    LoadBalancer   string `yaml:"load_balancer"`
    CircuitBreaker bool   `yaml:"circuit_breaker"`
    Watch          bool   `yaml:"watch"`
}

type RegistryConfig struct {
    Type string     `yaml:"type"`
    Etcd EtcdConfig `yaml:"etcd"`
}

type EtcdConfig struct {
    Endpoints   []string `yaml:"endpoints"`
    DialTimeout string   `yaml:"dial_timeout"`
    KeyPrefix   string   `yaml:"key_prefix"`
    LeaseTTL    int64    `yaml:"lease_ttl"`
}
```

## 加载配置

```go
import "RPCinGo/pkg/config"

// 从文件加载
cfg, err := config.LoadFromFile("config.yaml")
if err != nil {
    log.Fatal(err)
}

// 可选：用环境变量覆盖
if addr := os.Getenv("RPC_SERVER_ADDR"); addr != "" {
    cfg.Server.Address = addr
}
if etcd := os.Getenv("ETCD_ENDPOINTS"); etcd != "" {
    cfg.Registry.Etcd.Endpoints = strings.Split(etcd, ",")
}
```

## Builder 函数

### BuildServerOptions

```go
// pkg/config/config.go
func BuildServerOptions(cfg *Config) ([]server.Option, error) {
    codec, compress, err := parseCodecTypes(cfg.Server.Codec, cfg.Server.Compress)
    if err != nil {
        return nil, err
    }

    readTimeout, _ := time.ParseDuration(cfg.Server.ReadTimeout)
    writeTimeout, _ := time.ParseDuration(cfg.Server.WriteTimeout)
    heartbeatInterval, _ := time.ParseDuration(cfg.Server.HeartbeatInterval)

    opts := []server.Option{
        server.WithAddress(cfg.Server.Address),
        server.WithCodec(codec, compress),
        server.WithTimeout(readTimeout, writeTimeout),
        server.WithMaxConcurrent(cfg.Server.MaxConcurrent),
        server.WithWorkerPoolSize(cfg.Server.WorkerPoolSize),
    }

    if cfg.Server.EnableRegistry {
        reg, err := buildRegistry(&cfg.Registry)
        if err != nil {
            return nil, err
        }
        opts = append(opts,
            server.WithRegistry(reg, cfg.Server.ServiceName),
            server.WithServiceVersion(cfg.Server.ServiceVersion),
            server.WithHeartbeatInterval(int(heartbeatInterval.Seconds())),
        )
    }
    return opts, nil
}
```

### BuildClientOptions

```go
func BuildClientOptions(cfg *Config) ([]client.Option, error) {
    codec, compress, _ := parseCodecTypes(cfg.Client.Codec, cfg.Client.Compress)
    callTimeout, _ := time.ParseDuration(cfg.Client.CallTimeout)
    idleTimeout, _ := time.ParseDuration(cfg.Client.IdleTimeout)

    opts := []client.Option{
        client.WithCodec(codec),
        client.WithCompress(compress),
        client.WithCallTimeout(callTimeout),
        client.WithMaxConnections(cfg.Client.MaxConnections),
        client.WithMinConnections(cfg.Client.MinConnections),
        client.WithIdleTimeout(idleTimeout),
        client.WithCircuitBreaker(cfg.Client.CircuitBreaker),
        client.WithWatch(cfg.Client.Watch),
    }

    if cfg.Client.Mode == "discovery" {
        disc, err := buildDiscovery(&cfg.Registry)
        if err != nil {
            return nil, err
        }
        lb, err := parseLoadBalancer(cfg.Client.LoadBalancer)
        if err != nil {
            return nil, err
        }
        opts = append(opts,
            client.WithDiscovery(disc),
            client.WithLoadBalancer(lb),
        )
    } else {
        opts = append(opts, client.WithAddress(cfg.Client.Address))
    }
    return opts, nil
}
```

### parseCodecTypes（codec/compress 字符串转枚举）

```go
func parseCodecTypes(codecStr, compressStr string) (
    protocol.CodecType, protocol.CompressType, error) {

    var codecType protocol.CodecType
    switch strings.ToLower(codecStr) {
    case "json", "":
        codecType = protocol.CodecTypeJSON
    case "protobuf", "proto":
        codecType = protocol.CodecTypeProtobuf
    default:
        return 0, 0, fmt.Errorf("unknown codec: %s", codecStr)
    }

    var compressType protocol.CompressType
    switch strings.ToLower(compressStr) {
    case "none", "":
        compressType = protocol.CompressTypeNone
    case "gzip":
        compressType = protocol.CompressTypeGzip
    default:
        return 0, 0, fmt.Errorf("unknown compress: %s", compressStr)
    }

    return codecType, compressType, nil
}
```

### parseLoadBalancer（负载均衡字符串转实例）

```go
func parseLoadBalancer(name string) (loadbalancer.LoadBalancer, error) {
    switch strings.ToLower(name) {
    case "round_robin", "":
        return loadbalancer.NewRoundRobin(), nil
    case "random":
        return loadbalancer.NewRandom(), nil
    case "weighted":
        return loadbalancer.NewWeightedRoundRobin(), nil
    case "consistent", "consistent_hash":
        return loadbalancer.NewConsistentHash(150), nil
    default:
        return nil, fmt.Errorf("unknown load balancer: %s", name)
    }
}
```

## 完整使用示例

```go
package main

import (
    "context"
    "log"
    "RPCinGo/pkg/config"
    "RPCinGo/pkg/server"
)

func main() {
    // 加载配置
    cfg, err := config.LoadFromFile("config.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // 构建 Server Options
    opts, err := config.BuildServerOptions(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // 创建并启动服务端
    srv := server.NewServer(opts...)
    srv.RegisterService("UserService", &UserService{})
    srv.Start(context.Background())
}
```

## 不使用配置文件

如果不需要 YAML 配置，直接使用 Options 更简洁：

```go
srv := server.NewServer(
    server.WithAddress(":8080"),
    server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
    server.WithTimeout(5*time.Second, 5*time.Second),
    server.WithMaxConcurrent(1000),
)
```

## 相关文档

- [Server 概述](../server/overview.md) — Server Options 完整列表
- [Client 概述](../client/overview.md) — Client Options 完整列表
- [etcd 注册中心](../registry/etcd.md) — Registry 配置
