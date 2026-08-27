# 测试指南

## 概述

RPCinGo 的测试覆盖率约 63.5%，分布在各 `pkg/` 子包的 `*_test.go` 文件中。测试分为**单元测试**（使用 memory registry 等 mock）和**集成测试**（需要运行中的 etcd）。

## 测试文件分布

| 测试文件 | 类型 | 说明 |
|---------|------|------|
| `pkg/protocol/header_test.go` | 单元 | Header 编解码正确性 |
| `pkg/protocol/message_test.go` | 单元 | Request/Response 序列化 |
| `pkg/codec/codec_test.go` | 单元 | 编解码器接口行为 |
| `pkg/codec/json_test.go` | 单元 | JSON 编解码 |
| `pkg/codec/protobuf_test.go` | 单元 | Protobuf 编解码 + 类型检查 |
| `pkg/transport/transport_test.go` | 单元 | 传输接口契约 |
| `pkg/transport/tcp/client_test.go` | 集成 | TCP 客户端连接与发送 |
| `pkg/transport/tcp/server_test.go` | 集成 | TCP 服务端并发控制 |
| `pkg/transport/tcp/codec_test.go` | 单元 | ProtocolCodec 两阶段读写 |
| `pkg/pool/pool_test.go` | 集成 | 连接池 Get/Put/超时 |
| `pkg/pool/pool_manager_test.go` | 集成 | 多地址池并发安全 |
| `pkg/registry/memory/memory_test.go` | 单元 | Memory Registry 行为 |
| `pkg/registry/etcd/etcd_test.go` | 集成 | etcd 注册/发现（需运行 etcd） |
| `pkg/loadbalancer/balancer_test.go` | 单元 | 四种算法分布均匀性 |
| `pkg/circuitbreaker/breaker_test.go` | 单元 | 三状态转换、滑动窗口 |
| `pkg/ratelimiter/limiter_test.go` | 单元 | 令牌桶速率、滑动窗口 |
| `pkg/interceptor/interceptor_test.go` | 单元 | Chain 组合、各拦截器行为 |
| `pkg/server/server_test.go` | 集成 | 服务端启动/停止/请求路由 |
| `pkg/server/service_typed_test.go` | 集成 | 强类型 Protobuf 服务 |
| `pkg/client/client_typed_test.go` | 集成 | CallTyped 强类型调用 |

## 运行测试

```bash
# 运行全部测试
go test ./...

# 运行特定包测试
go test ./pkg/circuitbreaker/...
go test ./pkg/codec/...

# 带覆盖率
go test -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# 运行集成测试（需要 etcd）
etcd --data-dir default.etcd &
go test ./pkg/registry/etcd/...

# 并发测试（检测 race condition）
go test -race ./...

# 详细输出
go test -v ./pkg/circuitbreaker/...
```

## 单元测试编写规范

### 使用 Memory Registry 代替 etcd

```go
// 单元测试中使用 memory 实现，无需 etcd
reg := memory.NewMemoryRegistry()
reg.Register(ctx, registry.ServiceInstance{
    ID:      "test-instance",
    Service: "test-service",
    Address: "localhost",
    Port:    8080,
})

cli := client.New(
    client.WithDiscovery(reg, "test-service"),
)
```

### Mock 服务端

```go
func TestClientCall(t *testing.T) {
    // 启动轻量级测试服务端
    srv := server.New(server.WithAddress(":0")) // 随机端口
    srv.RegisterService("Echo", &EchoService{})
    go srv.Start()
    defer srv.Stop()

    // 获取实际端口
    addr := srv.Addr()
    cli := client.New(client.WithAddress(addr))
    // ...
}
```

### 熔断器测试示例

```go
func TestCircuitBreakerOpenState(t *testing.T) {
    cb := circuitbreaker.New(
        circuitbreaker.WithThreshold(5, 0.5), // 5次请求，50%失败率
        circuitbreaker.WithWindowSize(10, time.Second),
    )

    // 模拟失败请求
    for i := 0; i < 10; i++ {
        cb.Allow()
        cb.RecordFailure()
    }

    // 验证熔断打开
    assert.Equal(t, circuitbreaker.StateOpen, cb.State())
    assert.False(t, cb.Allow())
}
```

## 集成测试前提条件

etcd 集成测试需要本地运行 etcd：

```bash
# 方式一：直接启动
etcd --data-dir /tmp/test-etcd &

# 方式二：Docker
docker run -d -p 2379:2379 quay.io/coreos/etcd:v3.5.0 \
    /usr/local/bin/etcd --listen-client-urls http://0.0.0.0:2379 \
    --advertise-client-urls http://localhost:2379

# 方式三：使用项目内置 etcd 数据目录
etcd --data-dir default.etcd &
```

## 常见测试问题

| 问题 | 原因 | 解决方案 |
|------|------|---------|
| `connection refused` | 服务端未启动或端口冲突 | 使用 `:0` 随机端口，等待 `srv.Ready()` |
| Prometheus 注册冲突 | 测试间复用全局 Registry | 使用 `prometheus.NewRegistry()` 创建独立注册表 |
| etcd 测试超时 | etcd 未运行 | 跳过或使用 `t.Skip()` + 环境变量控制 |
| 熔断器状态干扰 | 测试间共用熔断器实例 | 每个测试创建独立熔断器实例 |
| Race condition | 连接池并发不安全 | 使用 `go test -race` 检测 |

## Source References

- `pkg/circuitbreaker/breaker_test.go`
- `pkg/codec/json_test.go`
- `pkg/registry/memory/memory_test.go`
- `pkg/registry/etcd/etcd_test.go`
- `pkg/pool/pool_test.go`
- `pkg/server/server_test.go`
- `test/`（集成测试工具）
