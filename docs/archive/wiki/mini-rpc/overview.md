# Mini-RPC 概述

## 目的

Mini-RPC 是 RPCinGo 的精简教学版本，约 1,500 行代码，去除了所有生产级特性，只保留 RPC 框架的核心骨架。目的是帮助理解 RPC 原理，作为阅读 `pkg/` 生产级代码的热身。

**源码位置**：`mini-rpc/`

## 目录结构

```
mini-rpc/
├── protocol/
│   └── message.go       ← 简化版消息格式（66 行）
├── codec/
│   ├── codec.go         ← 接口定义（34 行）
│   └── json.go          ← JSON 序列化
├── transport/
│   └── transport.go     ← 传输接口（27 行）
├── server/
│   └── server.go        ← 简单服务端（94 行）
├── client/
│   └── client.go        ← 简单客户端（103 行）
└── examples/simple/
    ├── server/main.go
    └── client/main.go
```

## 消息格式（简化）

**源码**：`mini-rpc/protocol/message.go`

```go
// 简化版：无固定头，无 Metadata，无超时控制
type Request struct {
    ID            uint64
    Service       string
    Method        string
    Args          []interface{} // 位置参数列表（非强类型）
}

type Response struct {
    ID     uint64
    Result interface{}
    Error  string // 简单字符串错误（非结构化错误码）
}
```

与生产版的对比：
- 无协议头（依赖 JSON 长度前缀或分隔符分帧）
- 无 `Metadata`、`Timeout`、`IsStream`、`ArgsCodec` 等字段
- `Error` 是简单字符串，无错误码体系

## 服务注册（简化）

**源码**：`mini-rpc/server/server.go`（94 行）

```go
// ServiceFunc：处理函数类型，接收位置参数列表
type ServiceFunc func(args []interface{}) (interface{}, error)

type Server struct {
    services  map[string]ServiceFunc // "ServiceName.MethodName" → handler
    transport transport.ServerTransport
}

func (s *Server) Register(service, method string, fn ServiceFunc) {
    key := service + "." + method
    s.services[key] = fn
}

func (s *Server) handleRequest(conn net.Conn) {
    // 1. 解码 JSON Request
    var req protocol.Request
    json.NewDecoder(conn).Decode(&req)

    // 2. 查找 handler
    key := req.Service + "." + req.Method
    fn, ok := s.services[key]

    // 3. 调用
    var resp protocol.Response
    resp.ID = req.ID
    if !ok {
        resp.Error = "method not found: " + key
    } else {
        result, err := fn(req.Args)
        if err != nil {
            resp.Error = err.Error()
        } else {
            resp.Result = result
        }
    }

    // 4. 编码响应
    json.NewEncoder(conn).Encode(resp)
}
```

与生产版对比：
- 无反射注册（手动 `Register("Calculator", "Add", fn)`）
- 无拦截器链
- 无并发控制（每连接一个 goroutine，无信号量限制）
- 无错误映射

## 客户端（简化）

**源码**：`mini-rpc/client/client.go`（103 行）

```go
type Client struct {
    conn      net.Conn
    codec     codec.StreamCodec
    mu        sync.Mutex  // 保护并发写
    pending   map[uint64]*Call // 等待中的异步调用
    requestID uint64          // 原子自增
}

type Call struct {
    ID     uint64
    Done   chan *Call
    Result interface{}
    Error  error
}

// 同步调用（阻塞等待响应）
func (c *Client) Call(service, method string, args ...interface{}) (interface{}, error) {
    call := c.Go(service, method, args, make(chan *Call, 1))
    result := <-call.Done
    return result.Result, result.Error
}

// 异步调用（立即返回 Call，通过 Done channel 接收结果）
func (c *Client) Go(service, method string, args []interface{},
    done chan *Call) *Call {

    call := &Call{
        ID:   atomic.AddUint64(&c.requestID, 1),
        Done: done,
    }

    c.mu.Lock()
    c.pending[call.ID] = call
    c.mu.Unlock()

    // 发送请求
    req := &protocol.Request{
        ID:      call.ID,
        Service: service,
        Method:  method,
        Args:    args,
    }
    c.codec.EncodeToWriter(c.conn, req)

    return call
}

// 后台 goroutine：读取响应，匹配并通知等待的 Call
func (c *Client) readResponses() {
    for {
        var resp protocol.Response
        if err := c.codec.DecodeFromReader(c.conn, &resp); err != nil {
            // 连接关闭，通知所有 pending 调用失败
            break
        }

        c.mu.Lock()
        call, ok := c.pending[resp.ID]
        delete(c.pending, resp.ID)
        c.mu.Unlock()

        if ok {
            call.Result = resp.Result
            if resp.Error != "" {
                call.Error = errors.New(resp.Error)
            }
            call.Done <- call
        }
    }
}
```

Mini-RPC 的客户端支持**异步调用**（`Go` 方法），这是生产版 client 没有直接暴露的特性（生产版通过 goroutine + channel 在内部实现）。

## 与生产版对比

| 特性 | Mini-RPC | pkg/（生产版）|
|------|----------|--------------|
| 代码量 | ~1,500 行 | ~10,900 行 |
| 协议头 | JSON 流（无固定头）| 20 字节固定头 |
| Codec | 仅 JSON | JSON + Protobuf + Gzip |
| 服务发现 | ❌ | etcd + memory |
| 负载均衡 | ❌ | 4 种算法 |
| 熔断/限流 | ❌ | 完整实现 |
| 连接池 | ❌ | 完整实现 |
| 拦截器 | ❌ | 完整链式拦截器 |
| 错误体系 | 字符串 | 结构化错误码 |
| 异步调用 | ✅（`Go` 方法）| 内部实现，外部同步 |
| 服务注册 | 手动（`ServiceFunc`）| 反射自动 |
| 并发控制 | 无 | 双层信号量 |
| 测试覆盖 | 有单元测试 | 63.5% |

## 运行示例

```bash
# 终端 1
go run mini-rpc/examples/simple/server/main.go

# 终端 2
go run mini-rpc/examples/simple/client/main.go
```

## 推荐阅读顺序

先读 Mini-RPC 理解框架骨架，再读生产版理解每个特性如何完善：

```
Mini-RPC 骨架                    生产版对应位置
───────────────────────────────────────────────
mini-rpc/protocol/message.go  → pkg/protocol/
mini-rpc/codec/               → pkg/codec/
mini-rpc/transport/           → pkg/transport/tcp/
mini-rpc/server/server.go     → pkg/server/
mini-rpc/client/client.go     → pkg/client/
─                             → pkg/pool/         （连接池）
─                             → pkg/registry/     （服务发现）
─                             → pkg/loadbalancer/ （负载均衡）
─                             → pkg/circuitbreaker/（熔断）
─                             → pkg/ratelimiter/  （限流）
─                             → pkg/interceptor/  （拦截器）
```

## 相关文档

- [整体架构](../architecture/overview.md)
- [快速开始](quick-start.md)
- [数据流](../architecture/data-flow.md) — 生产版的完整请求链路
