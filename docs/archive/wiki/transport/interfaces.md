# 传输接口

## 概述

`pkg/transport` 定义了客户端与服务端传输层的抽象接口，当前唯一实现是 TCP（`pkg/transport/tcp/`），接口设计预留了未来添加其他传输协议（如 QUIC、Unix Socket）的能力。

**源码位置**：`pkg/transport/transport.go`、`pkg/transport/options.go`

## 接口定义

### ClientTransport

```go
// pkg/transport/transport.go
type ClientTransport interface {
    // 建立连接
    Dial(ctx context.Context, addr string) error
    // 发送请求并等待响应（支持同一连接上的并发调用）
    SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
    // 关闭连接
    Close() error
    // 检查连接是否存活
    IsConnected() bool
    // 返回本地地址
    LocalAddr() net.Addr
    // 返回远端地址
    RemoteAddr() net.Addr
}
```

`SendRequest` 是多路复用模型的核心入口：多个 goroutine 可以在同一连接上并发调用，内部通过 `RequestID` 将响应路由回各自的调用方，无需每个请求独占一条连接。

### ServerTransport

```go
type ServerTransport interface {
    // 开始监听指定地址
    Listen(address string) error
    // 开始服务（阻塞，接受连接并处理）
    Serve(handler Handler) error
    // 关闭监听器和所有连接
    Close() error
    // 返回监听地址
    Addr() string
}
```

### Handler（请求处理回调）

```go
// 服务端处理 RPC 请求的函数类型
type Handler func(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
```

`Server`（`pkg/server`）将自身的 `HandleRequest` 方法作为 `Handler` 传给 `ServerTransport`，实现传输层与业务层解耦。

### Connection

```go
type Connection interface {
    io.ReadWriter
    io.Closer
    LocalAddr() net.Addr
    RemoteAddr() net.Addr
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error
}
```

## 配置选项

### ClientOptions

```go
// pkg/transport/options.go
type ClientOptions struct {
    DialTimeout   time.Duration // 连接建立超时，默认 5s
    KeepAlive     time.Duration // TCP KeepAlive 间隔，默认 30s
    ReadTimeout   time.Duration // 读超时，默认 30s
    WriteTimeout  time.Duration // 写超时，默认 30s
    BufferSize    int           // 读写缓冲大小，默认 4096 字节
    MaxRetries    int           // 连接失败最大重试次数，默认 3
    RetryInterval time.Duration // 重试间隔，默认 1s
}

// 选项函数（Option 模式）
func WithDialTimeout(d time.Duration) ClientOption
func WithKeepAlive(d time.Duration) ClientOption
func WithReadTimeout(d time.Duration) ClientOption
func WithWriteTimeout(d time.Duration) ClientOption
func WithBufferSize(size int) ClientOption
```

### ServerOptions

```go
type ServerOptions struct {
    ReadTimeout          time.Duration // 读超时，默认 30s
    WriteTimeout         time.Duration // 写超时，默认 30s
    MaxConcurrentRequests int          // 最大并发请求数，默认 10000
    WorkerPoolSize       int           // goroutine 池大小，默认 100
    BufferSize           int           // 读写缓冲大小，默认 4096 字节
    MaxRequestBodySize   int64         // 最大请求体大小，默认 10MB
    MaxConnections       int           // 最大连接数，默认 10000
}

func WithServerReadTimeout(d time.Duration) ServerOption
func WithServerWriteTimeout(d time.Duration) ServerOption
func WithMaxConcurrentRequests(n int) ServerOption
func WithWorkerPoolSize(n int) ServerOption
func WithMaxConnections(n int) ServerOption
```

## 架构中的位置

```
pkg/server ──── 实现 Handler ────► pkg/transport（接口）
                                        │
                                        ▼
                                  pkg/transport/tcp
                                  （唯一实现）
```

`pkg/client` 通过 `pkg/pool`（连接池）间接使用 `ClientTransport`，不直接调用传输接口。

## 相关文档

- [TCP 传输](tcp.md) — 接口的 TCP 实现
- [连接池](connection-pool.md) — 对 ClientTransport 的池化管理
- [Server 概述](../server/overview.md) — Handler 注册
