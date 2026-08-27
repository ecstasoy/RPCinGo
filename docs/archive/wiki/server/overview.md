# Server 概述

## 职责

`pkg/server` 是 RPC 服务端的核心，负责监听 TCP 端口、处理请求、路由到 handler、管理服务注册与心跳。

**源码位置**：`pkg/server/server.go`（197 行）、`pkg/server/options.go`（87 行）

## 核心结构

```go
type Server struct {
    opts serverOptions

    transport *tcp.Server           // TCP 传输层
    registry  *ServiceRegistry      // 方法路由表
    chain     *interceptor.Chain    // 拦截器链
    reg       registry.Registry     // 服务注册中心（可选）
    codec     codec.Codec           // 编解码器（含压缩）

    // 生命周期管理
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}
```

## 配置项（serverOptions）

**源码**：`pkg/server/options.go`（87 行）

```go
type serverOptions struct {
    address           string
    codec             protocol.CodecType
    compress          protocol.CompressType
    readTimeout       time.Duration
    writeTimeout      time.Duration
    maxConcurrent     int
    workerPoolSize    int
    interceptors      []interceptor.Interceptor

    // 服务注册相关
    registryEnabled   bool
    registry          registry.Registry
    serviceName       string
    serviceVersion    string
    serviceWeight     int
    serviceMetadata   map[string]string
    heartbeatInterval int // 秒
}

// 默认值
func defaultServerOptions() serverOptions {
    return serverOptions{
        address:           ":8080",
        codec:             protocol.CodecTypeJSON,
        compress:          protocol.CompressTypeNone,
        readTimeout:       30 * time.Second,
        writeTimeout:      30 * time.Second,
        maxConcurrent:     10000,
        workerPoolSize:    100,
        heartbeatInterval: 10,
    }
}
```

| Option 函数 | 说明 |
|-------------|------|
| `WithAddress(addr)` | 监听地址，如 `":8080"` 或 `"127.0.0.1:8080"` |
| `WithCodec(codec, compress)` | 序列化格式与压缩算法 |
| `WithTimeout(read, write)` | 读写超时 |
| `WithMaxConcurrent(n)` | 最大并发请求数（超限返回 ResourceExhausted）|
| `WithWorkerPoolSize(n)` | goroutine 池大小 |
| `WithInterceptors(...)` | 拦截器列表（Recovery 建议放第一位）|
| `WithRegistry(reg, name)` | 服务注册中心和服务名 |
| `WithServiceVersion(v)` | 服务版本（用于多版本路由）|
| `WithServiceWeight(w)` | 实例权重（用于加权负载均衡）|
| `WithHeartbeatInterval(s)` | 心跳间隔（秒） |
| `WithLogger(l)` | 注入 `logger.Logger`（来自 `pkg/logger`），不传则默认 `logger.New()`（slog 输出）|

## 生命周期

```
NewServer(options...)
    │
    ├── 应用 Options，设置默认值
    ├── 初始化 ServiceRegistry
    ├── 初始化 InterceptorChain
    ├── 选择 Codec（按 options.codec + compress 构建，含压缩时用 CompressedCodec）
    └── 初始化 TCPServer

Start(ctx) ── 阻塞
    │
    ├── 若 registryEnabled：
    │   ├── reg.Register(ctx, &ServiceInstance{
    │   │       Service: serviceName,
    │   │       Address: host,
    │   │       Port:    port,
    │   │       Version: serviceVersion,
    │   │       Weight:  serviceWeight,
    │   │       Status:  InstanceStatusUp,
    │   │   })
    │   └── 启动心跳 goroutine：
    │       每 heartbeatInterval 秒调用 reg.Heartbeat(ctx, serviceName, instanceID)
    │
    └── tcpServer.Listen(address)
        tcpServer.Serve(s.HandleRequest) ← 阻塞

Stop()
    ├── stopHeartbeatOne.Do(close(stopHeartbeat))
    │   // sync.Once 保护，幂等关闭，防止 double-Stop() 引发 panic
    ├── tcpServer.Close()   // 关闭监听器，停止接受新连接
    ├── wg.Wait()           // 等待进行中的请求完成
    └── 若 registryEnabled：
        reg.Deregister(ctx, serviceName, instanceID)
```

## 请求处理流程（HandleRequest）

```go
func (s *Server) HandleRequest(ctx context.Context,
    req *protocol.Request) (*protocol.Response, error) {

    // 通过拦截器链调用实际 handler
    result, err := s.chain.Execute(ctx, req,
        func(ctx context.Context, req *protocol.Request) (interface{}, error) {
            return s.registry.Invoke(ctx, req)
        })

    // 构建 Response
    resp := &protocol.Response{
        ID:         req.ID,
        ServerTime: time.Now().UnixMilli(),
    }

    if err != nil {
        resp.Error = mapError(err) // server/error_map.go
    } else {
        resp.Data = result
    }
    return resp, nil
}
```

`mapError` 将各种 Go error 转换为协议 `*protocol.Error`（见 [错误码](../protocol/error-codes.md)）。

## 完整启动示例

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "RPCinGo/pkg/interceptor"
    "RPCinGo/pkg/protocol"
    "RPCinGo/pkg/ratelimiter"
    "RPCinGo/pkg/registry/etcd"
    "RPCinGo/pkg/server"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // 初始化注册中心
    reg, _ := etcd.NewRegistry(
        etcd.WithEndpoints("localhost:2379"),
    )

    // 初始化限流器
    limiter := ratelimiter.NewTokenBucket(10000, 500)

    // 创建 Server
    srv := server.NewServer(
        server.WithAddress(":8080"),
        server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
        server.WithTimeout(30*time.Second, 30*time.Second),
        server.WithMaxConcurrent(5000),
        server.WithRegistry(reg, "UserService"),
        server.WithServiceVersion("1.0.0"),
        server.WithServiceWeight(1),
        server.WithHeartbeatInterval(10),
        server.WithInterceptors(
            interceptor.NewRecoveryInterceptor(),    // 最外层
            interceptor.NewLoggingInterceptor(nil),
            interceptor.NewMetricsInterceptor(),
            interceptor.NewRateLimitInterceptor(limiter),
        ),
    )

    // 注册服务
    srv.RegisterService("UserService", &UserService{})

    // 暴露 Prometheus 指标
    go func() {
        http.Handle("/metrics", promhttp.Handler())
        http.ListenAndServe(":9090", nil)
    }()

    // 优雅退出
    go func() {
        sig := make(chan os.Signal, 1)
        signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
        <-sig
        log.Println("Shutting down...")
        srv.Stop()
    }()

    log.Println("Server starting on :8080")
    srv.Start(context.Background())
}
```

## 相关文档

- [服务注册](service-registration.md) — 方法注册机制
- [拦截器链](interceptors.md) — 拦截器详解
- [日志模块](../logger/overview.md) — Logger 接口与自定义实现
- [TCP 传输](../transport/tcp.md) — 底层网络实现
- [etcd 注册中心](../registry/etcd.md) — 服务注册与心跳
- [Prometheus 指标](../observability/metrics.md) — 监控集成
