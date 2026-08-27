# 固定地址模式

## 概述

Fixed 模式直接连接到指定地址的 RPC 服务端，不经过服务发现。适用于：

- 开发和测试环境
- 服务地址固定不变的场景
- 单机部署或已知地址的微服务

**源码位置**：`pkg/client/client.go`

## 创建客户端

```go
cli, err := client.NewClient("127.0.0.1:8080",
    client.WithCodec(protocol.CodecTypeJSON),
    client.WithCompress(protocol.CompressTypeNone),
    client.WithCallTimeout(5 * time.Second),
    client.WithMaxConnections(100),
    client.WithMinConnections(10),
)
if err != nil {
    log.Fatal(err)
}
defer cli.Close()
```

## 调用流程

```
cli.Call(ctx, service, method, args)
    │
    ├── 1. 构建 Request{ID, Service, Method, Args, Timeout}
    │
    ├── 2. pool.Get(ctx) → net.Conn（从连接池获取）
    │
    ├── 3. Encode(Request) → [Header | Body]
    │
    ├── 4. conn.Write([Header | Body])
    │
    ├── 5. conn.Read() → [Header | Body]
    │
    ├── 6. Decode(Response) → result / error
    │
    ├── 7. pool.Put(conn)（归还连接）
    │
    └── 8. 返回 (result, error)
```

## 超时控制

超时通过 `context.WithTimeout` 传递：

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

result, err := cli.Call(ctx, "Calculator", "Add", args)
```

当 `ctx` 超时时：
- 若请求尚未发出：直接返回 `context.DeadlineExceeded`
- 若请求已发出等待响应：TCP 读超时触发，返回 `context.DeadlineExceeded`

## 连接池行为

Fixed 模式绑定一个固定地址的连接池：

```
ConnectionPool("127.0.0.1:8080")
    ├── 预创建 minConnections 个连接（MinSize）
    ├── 按需创建至 MaxConnections（MaxSize）
    ├── 每个连接复用，call 完毕后归还
    └── 后台定期清理超时空闲连接
```

连接池容量不足时（所有连接均借出且已达 MaxSize），`Get()` 阻塞等待，直到有连接归还或 ctx 超时。

## 与 Discovery 模式对比

| 特性 | Fixed 模式 | Discovery 模式 |
|------|-----------|----------------|
| 目标地址 | 固定 | 动态发现 |
| 负载均衡 | 无 | 支持 |
| 熔断 | 无 | 支持 |
| 适用场景 | 开发/测试/固定部署 | 生产微服务 |
| 复杂度 | 低 | 高 |

## 相关文档

- [Client 概述](overview.md)
- [服务发现模式](discovery-mode.md)
- [连接池](../transport/connection-pool.md)
