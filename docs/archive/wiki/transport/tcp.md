# TCP 传输

## 概述

`pkg/transport/tcp` 是 RPCinGo 唯一的传输层实现，提供高性能 TCP 客户端和服务端，内置连接管理、并发控制和协议编解码。

**源码位置**：`pkg/transport/tcp/`（client.go 261行、server.go 330行、codec.go 204行）

## TCP 客户端

**源码**：`pkg/transport/tcp/client.go`

### 多路复用模型

同一连接上多个 goroutine 可以**并发**发送请求，每个请求通过 `RequestID` 在 pending map 中注册，专属 `readLoop` goroutine 持续读取响应并按 ID 路由回各自的调用方。

```
goroutine A ──┐
goroutine B ──┼──► writeMu（串行写）──► conn ──► readLoop ──► pending map ──► 各自 channel
goroutine C ──┘
```

```go
type Client struct {
    conn      net.Conn
    Codec     *ProtocolCodec
    mu        sync.RWMutex   // 保护 connected / conn 字段

    writeMu   sync.Mutex              // 串行化并发写，防止帧交错
    pendingMu sync.Mutex              // 保护 pending map
    pending   map[uint64]*pendingCall // requestID → 等待响应的调用
    closeCh   chan struct{}            // 连接关闭信号
    closeOnce sync.Once
}

type pendingCall struct {
    done     chan struct{}
    respBody []byte // 解压后的响应 body bytes
    err      error
}
```

### 连接建立与 readLoop 启动

`Dial` 建立 TCP 连接后，立即初始化 pending map 并启动 `readLoop` goroutine：

```go
c.pending = make(map[uint64]*pendingCall)
c.closeCh  = make(chan struct{})
go c.readLoop(conn) // 传入 conn 避免与 Close() 的 nil 赋值竞争
```

`SetNoDelay(true)` 禁用 Nagle 算法，确保每个请求立即发送，降低延迟。

### SendRequest（多路复用核心）

```go
func (c *Client) SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
    call := &pendingCall{done: make(chan struct{})}

    // 注册 pending（在写之前，防止极快响应丢失）
    c.pendingMu.Lock()
    c.pending[req.ID] = call
    c.pendingMu.Unlock()

    // 串行写（多个 goroutine 安全）
    c.writeMu.Lock()
    err := c.Codec.WriteRequest(c.conn, req)
    c.writeMu.Unlock()

    // 等待响应、ctx 超时或连接关闭
    select {
    case <-call.done:
        return c.Codec.DecodeResponse(call.respBody)
    case <-ctx.Done():
        // 超时取消：从 pending 删除，丢弃迟到的响应
        c.pendingMu.Lock()
        delete(c.pending, req.ID)
        c.pendingMu.Unlock()
        return nil, ctx.Err()
    case <-c.closeCh:
        return nil, fmt.Errorf("connection closed")
    }
}
```

### readLoop

```go
func (c *Client) readLoop(conn net.Conn) {
    defer c.closeWithError(fmt.Errorf("connection closed"))
    for {
        header, bodyBytes, err := c.Codec.DecodeFromReader(conn)
        if err != nil {
            return
        }
        c.pendingMu.Lock()
        call, ok := c.pending[header.RequestID]
        if ok {
            delete(c.pending, header.RequestID)
        }
        c.pendingMu.Unlock()
        if ok {
            call.respBody = bodyBytes
            close(call.done)
        }
        // 找不到对应 call（已超时取消）则静默丢弃
    }
}
```

### 连接关闭

`Close()` 关闭底层 conn（令 `readLoop` 的 `DecodeFromReader` 返回错误），同时调用 `closeWithError` 主动唤醒所有 pending caller，避免它们永久阻塞：

```go
func (c *Client) closeWithError(err error) {
    c.closeOnce.Do(func() {
        close(c.closeCh)
        c.pendingMu.Lock()
        for id, call := range c.pending {
            call.err = err
            close(call.done)
            delete(c.pending, id)
        }
        c.pendingMu.Unlock()
    })
}
```

## TCP 服务端

**源码**：`pkg/transport/tcp/server.go`

### 服务端并发模型（多路复用）

每个连接上**读**和**写**完全分离：读循环快速派发请求给 handler goroutine，writer goroutine 通过 channel 串行写响应，防止多个 handler 并发写导致帧交错。

```
Accept() ← 主 goroutine
    │
    └── 每个连接
            │
            ├── read loop（串行读，快速派发）
            │       │
            │       └── go handler goroutine（受 reqSem 限制）
            │               │
            │               └── handler(ctx, req) → writeCh <- resp
            │
            └── writer goroutine（从 writeCh 串行写回 conn）
```

关键结构：

```go
writeCh := make(chan *protocol.Response, 64)
var handlersWg sync.WaitGroup

// writer goroutine：防止帧交错
go func() {
    for resp := range writeCh {
        s.codec.WriteResponse(conn, resp)
    }
}()

// read loop：读一个，派发一个
for {
    header, req, _ := s.codec.ReadRequest(conn)

    handlersWg.Add(1)
    go func(header, req) {
        defer handlersWg.Done()
        resp, _ := s.handler(ctx, req)
        writeCh <- resp   // handler 完成后投入写队列
    }(header, req)
}

// 等所有 handler 把响应写入 writeCh，再关闭
handlersWg.Wait()
close(writeCh)
```

`handlersWg.Wait()` 保证 `close(writeCh)` 时不会有 handler goroutine 仍在尝试发送，避免 `send on closed channel` panic。

### 双层信号量控制

```go
// 连接级别限制
select {
case s.connSemaphore <- struct{}{}:
    go s.handleConnection(conn)
default:
    conn.Close() // 连接数超限，直接拒绝
}

// 请求级别限制（在 read loop 内）
select {
case s.reqSemaphore <- struct{}{}:
    // 派发 handler goroutine
default:
    writeCh <- protocol.NewErrorResponse(header.RequestID,
        protocol.NewError(protocol.ErrorCodeUnavailable, "too many concurrent requests"))
}
```

### TCP 服务端配置

```go
// pkg/transport/tcp/server.go 中的默认值
const (
    defaultReadTimeout          = 30 * time.Second
    defaultWriteTimeout         = 30 * time.Second
    defaultMaxConcurrentRequests = 10000
    defaultWorkerPoolSize       = 100
    defaultBufferSize           = 4096
    defaultMaxRequestBodySize   = 10 * 1024 * 1024 // 10MB
    defaultMaxConnections       = 10000
)
```

### 服务端统计

```go
type ServerStats struct {
    TotalConnections  int64  // 总建立连接数（原子）
    ActiveConnections int64  // 当前活跃连接数（原子）
    TotalRequests     int64  // 总处理请求数（原子）
    FailedRequests    int64  // 失败请求数（原子）
}
```

可通过 `server.Stats()` 方法获取统计数据，集成到监控系统。

## ProtocolCodec（协议编解码器）

**源码**：`pkg/transport/tcp/codec.go`（204 行）

`ProtocolCodec` 是传输层与协议层之间的桥接，负责将 `*protocol.Request`/`*protocol.Response` 与网络字节流互转。

### 关键方法

```go
type ProtocolCodec struct{}

// 将 Request 编码为 [Header(20B) | Body] 字节
func (c *ProtocolCodec) EncodeRequest(req *protocol.Request, codec codec.Codec) ([]byte, error)

// 将 [Header | Body] 字节解码为 Request
func (c *ProtocolCodec) DecodeRequest(header *protocol.Header, body []byte, c codec.Codec) (*protocol.Request, error)

// 直接向 io.Writer 写入编码后的 Request（内部用 writeFull 循环写，保证大消息不被截断）
func (c *ProtocolCodec) WriteRequest(w io.Writer, req *protocol.Request, codec codec.Codec) error

// 从 io.Reader 读取并解码 Request（两阶段：先读 Header，再读 Body）
func (c *ProtocolCodec) ReadRequest(r io.Reader, codec codec.Codec) (*protocol.Request, error)

// Response 的对称方法
func (c *ProtocolCodec) EncodeResponse(resp *protocol.Response, codec codec.Codec) ([]byte, error)
func (c *ProtocolCodec) DecodeResponse(header *protocol.Header, body []byte, c codec.Codec) (*protocol.Response, error)
func (c *ProtocolCodec) WriteResponse(w io.Writer, resp *protocol.Response, codec codec.Codec) error // 同样使用 writeFull
func (c *ProtocolCodec) ReadResponse(r io.Reader, codec codec.Codec) (*protocol.Response, error)
```

### 两阶段读取实现

```go
func (c *ProtocolCodec) ReadRequest(r io.Reader, codec codec.Codec) (*protocol.Request, error) {
    // 阶段 1：读取固定 20 字节 Header
    headerBuf := make([]byte, protocol.HeaderSize)
    if _, err := io.ReadFull(r, headerBuf); err != nil {
        return nil, err
    }
    header, err := decodeHeader(headerBuf)
    if err != nil {
        return nil, err
    }

    // 防止超大请求体导致 OOM
    if header.BodyLength > maxBodySize {
        return nil, fmt.Errorf("request body too large: %d > %d", header.BodyLength, maxBodySize)
    }

    // 阶段 2：按 BodyLength 读取变长 Body
    body := make([]byte, header.BodyLength)
    if _, err := io.ReadFull(r, body); err != nil {
        return nil, err
    }

    // 如有压缩，先解压
    if header.Compress != protocol.CompressTypeNone {
        compressor := codec.GetCompressor(header.Compress)
        body, err = compressor.Decompress(body)
        if err != nil {
            return nil, err
        }
    }

    // 用对应 Codec 解码 Body
    req := &protocol.Request{}
    if err := codec.Decode(body, req); err != nil {
        return nil, err
    }
    return req, nil
}
```

## 注意事项

- **写入不完整**：`net.Conn.Write` 不保证一次写完全部字节（内核缓冲区满时会短写）。`WriteRequest` 和 `WriteResponse` 内部使用 `writeFull` 循环写入，确保大消息在高并发下不被截断。
- **连接断开检测**：`SetReadDeadline` 超时或 `io.ReadFull` 返回 `io.EOF` 时，readLoop 退出并触发 `closeWithError`，唤醒所有 pending caller。
- **writeFull 签名**：`writeFull(w io.Writer, b []byte) error`，接受 `io.Writer` 而非 `net.Conn`，可在 client 和 codec 中共用。

## 相关文档

- [传输接口](interfaces.md) — 接口定义
- [连接池](connection-pool.md) — TCPClient 的池化管理
- [协议头](../protocol/header.md) — 20 字节头部格式
- [数据流](../architecture/data-flow.md) — 端到端处理流程
