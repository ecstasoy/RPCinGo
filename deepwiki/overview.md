# 概述

## 项目目的

RPCinGo 是一个用 Go 实现的**生产级 RPC 框架**，面向以下场景：

- 微服务间高性能通信（165,000+ QPS，<1ms 延迟）
- 需要服务发现、负载均衡、熔断、限流的分布式系统
- 希望深入理解 RPC 框架原理的学习者（附 mini-rpc 教学版本，~1,500 行）

**主要用户群体**：Go 后端工程师、微服务架构师、RPC 框架学习者。

## 仓库目录布局

| 目录 | 用途 |
|------|------|
| `pkg/` | 生产级 RPC 框架核心代码（~10,900 行） |
| `mini-rpc/` | 教学精简版本（~1,500 行），无服务发现/熔断等高级功能 |
| `examples/` | `calculator/`（Protobuf 强类型）和 `microservice/`（etcd 多节点）示例 |
| `docs/` | 架构说明文档（ARCHITECTURE_EXPLAINED.md 等） |
| `wiki/` | 原始知识库（35 个 Markdown 文档，已迁移至 deepwiki/） |
| `proto/` | Protobuf 定义文件 |
| `scripts/` | 构建与代码生成脚本 |
| `configs/` | YAML 配置文件模板 |
| `test/` | 集成测试工具 |
| `default.etcd/` | 本地 etcd 数据目录（开发用） |

## 入口点

| 场景 | 入口 |
|------|------|
| 启动服务端 | `server.New(opts...)` → `server.Start()` |
| 创建客户端（固定地址） | `client.New(client.WithAddress("host:port"))` |
| 创建客户端（服务发现） | `client.New(client.WithDiscovery(discovery, "service-name"))` |
| YAML 配置加载 | `config.Load("config.yaml")` → `config.BuildServerOptions()` |
| Calculator 示例 | `examples/calculator/` |
| Microservice 示例 | `examples/microservice/` |

## 关键工件

| 文件 | 说明 |
|------|------|
| `pkg/protocol/header.go` | 20 字节固定协议头，Magic=0xCAFE |
| `pkg/codec/` | JSON / Protobuf 编解码器 + Gzip 装饰器 |
| `pkg/transport/tcp/` | TCP 客户端/服务端，两阶段头/体读取 |
| `pkg/server/server.go` | 服务注册、拦截器链 |
| `pkg/client/client.go` | 双模式客户端，Call / CallTyped |
| `pkg/circuitbreaker/breaker.go` | 三状态熔断器 + 滑动窗口 |
| `go.mod` | 模块名 `RPCinGo`，Go 1.24.5 |

## 关键工作流

```bash
# 1. 生成 Protobuf 文件
bash scripts/gen-example-proto.sh

# 2. 启动本地 etcd（微服务示例需要）
etcd --data-dir default.etcd &

# 3. 运行计算器示例（服务端）
go run examples/calculator/server/main.go

# 4. 运行计算器示例（客户端）
go run examples/calculator/client/main.go

# 5. 运行全部测试
go test ./...
```

## 示例代码片段

**最小化服务端**：

```go
srv := server.New(
    server.WithAddress(":8080"),
    server.WithCodec(codec.NewJSONCodec()),
)
srv.RegisterService("Calculator", &CalculatorService{})
srv.Start()
```

**最小化客户端**：

```go
cli := client.New(
    client.WithAddress("localhost:8080"),
    client.WithCodec(codec.NewJSONCodec()),
)
resp, err := cli.Call(ctx, "Calculator", "Add", req)
```

## 从哪里开始

- **新手**：先读 `deepwiki/overview.md`（本文），再看 `deepwiki/guides/quick-start.md`（待补充）
- **了解架构**：`deepwiki/architecture.md`
- **深入某模块**：`deepwiki/modules/<module>.md`
- **查看示例**：`examples/calculator/` 或 `examples/microservice/`

## 如何导航

deepwiki 按以下方式组织：

```
deepwiki/
├── overview.md          # 本文：项目全景
├── architecture.md      # 系统架构与组件关系
├── data-flow.md         # 请求全链路数据流
├── dependencies.md      # 外部依赖与 go.mod
├── glossary.md          # 术语表
├── modules/             # 各包的深度文档
│   ├── protocol.md
│   ├── codec.md
│   ├── transport.md
│   ├── server.md
│   ├── client.md
│   ├── registry.md
│   ├── loadbalancer.md
│   ├── circuitbreaker.md
│   ├── ratelimiter.md
│   ├── interceptor.md
│   ├── pool.md
│   ├── config.md
│   └── mini-rpc.md
└── guides/              # 专题指南
    ├── configuration.md
    ├── telemetry.md
    ├── testing.md
    └── security.md
```

## 常见陷阱

- **CallTyped 仅支持 Protobuf**：若用 JSON 调用 `CallTyped` 会 panic，因为它要求参数实现 `proto.Message`
- **Discovery 模式需要运行中的 etcd**：Fixed 模式无此依赖
- **熔断器独立于每个地址**：多实例时，单实例触发熔断不影响其他实例
- **mini-rpc 不可用于生产**：缺少连接池、服务发现、熔断等关键功能
- **Protobuf 编码使用 4 字节长度前缀帧**：与 JSON 的处理方式不同，混用时需注意

## Source References

- `pkg/server/server.go`
- `pkg/client/client.go`
- `pkg/protocol/header.go`
- `pkg/codec/`
- `pkg/transport/tcp/`
- `go.mod`
- `examples/calculator/`
- `examples/microservice/`
- `wiki/README.md`
- `wiki/getting-started/quick-start.md`
