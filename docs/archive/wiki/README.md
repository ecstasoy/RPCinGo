# RPCinGo 知识库

RPCinGo 是一个用 Go 编写的高性能、生产级 RPC 框架，从 Java 版本移植并扩展，提供完整的微服务通信解决方案。

| 属性 | 值 |
|------|-----|
| 语言 | Go 1.24.5 |
| 核心代码 | ~10,900 行（pkg/） |
| 依赖 | etcd v3.6.7, Protobuf v1.36.11, Prometheus v1.23.2, yaml.v3 |
| 性能 | 165k+ QPS，<1ms 延迟，~2MB 内存 |

---

## 目录

### 快速入门
| 文档 | 说明 |
|------|------|
| [快速开始](getting-started/quick-start.md) | 5 分钟跑通第一个 RPC 调用 |
| [Calculator 示例](getting-started/calculator-example.md) | 基于 Protobuf 的强类型调用 |
| [微服务示例](getting-started/microservice-example.md) | etcd 服务发现 + 负载均衡完整示例 |

### 架构
| 文档 | 说明 |
|------|------|
| [整体架构](architecture/overview.md) | 分层架构与组件关系图 |
| [数据流](architecture/data-flow.md) | 请求从发起到响应的完整链路 |
| [设计模式](architecture/design-patterns.md) | 框架中使用的关键设计模式 |

### 可视化图表
| 图表 | 说明 |
|------|------|
| [整体架构图](diagrams/architecture.excalidraw) | 五层架构 · 各层核心组件 ([PNG 预览](diagrams/architecture.png)) |
| [请求数据流](diagrams/dataflow.excalidraw) | 客户端→编码→TCP→服务端→Handler 完整链路 ([PNG 预览](diagrams/dataflow.png)) |
| [代码骨架](diagrams/skeleton.excalidraw) | pkg/ 目录树 · 每个包的核心职责 ([PNG 预览](diagrams/skeleton.png)) |
| [服务发现流程](diagrams/discovery-pipeline.excalidraw) | Watch goroutine · LB · 熔断器 · 连接池调用链 ([PNG 预览](diagrams/discovery-pipeline.png)) |

### 协议层 `pkg/protocol`
| 文档 | 说明 |
|------|------|
| [消息格式](protocol/message-format.md) | Request / Response 完整结构 |
| [协议头](protocol/header.md) | 20 字节固定头部逐字段详解 |
| [编解码类型](protocol/codec-types.md) | 支持的序列化格式与压缩算法 |
| [错误码](protocol/error-codes.md) | 完整错误码体系与双向映射 |
| [Metadata](protocol/metadata.md) | 请求元数据与标准键定义 |

### 编解码层 `pkg/codec`
| 文档 | 说明 |
|------|------|
| [Codec 概述](codec/overview.md) | 注册表机制、StreamCodec、压缩装饰器 |
| [JSON Codec](codec/json.md) | JSON 序列化与 Payload 自适应处理 |
| [Protobuf Codec](codec/protobuf.md) | Protobuf 序列化与 4 字节帧格式 |

### 传输层 `pkg/transport`
| 文档 | 说明 |
|------|------|
| [传输接口](transport/interfaces.md) | ClientTransport / ServerTransport 接口定义 |
| [TCP 传输](transport/tcp.md) | TCP 客户端/服务端实现细节 |
| [连接池](transport/connection-pool.md) | 池化管理、工厂模式、健康检查、统计 |

### 服务端 `pkg/server`
| 文档 | 说明 |
|------|------|
| [Server 概述](server/overview.md) | 生命周期、请求处理流程、配置项 |
| [服务注册](server/service-registration.md) | 三种方法签名与反射自动注册 |
| [拦截器链](server/interceptors.md) | 内置拦截器详解与自定义拦截器 |

### 客户端 `pkg/client`
| 文档 | 说明 |
|------|------|
| [Client 概述](client/overview.md) | Fixed / Discovery 双模式，Call / CallTyped |
| [固定地址模式](client/fixed-mode.md) | 直连单服务器 |
| [服务发现模式](client/discovery-mode.md) | 动态发现 + 负载均衡 + 熔断 + Watch |

### 注册中心 `pkg/registry`
| 文档 | 说明 |
|------|------|
| [Registry 概述](registry/overview.md) | 接口定义、ServiceInstance、Watcher 事件 |
| [etcd 实现](registry/etcd.md) | 租约注册、KeepAlive、Watch 机制 |
| [内存实现](registry/memory.md) | 测试/单机场景 |

### 负载均衡 `pkg/loadbalancer`
| 文档 | 说明 |
|------|------|
| [负载均衡概述](loadbalancer/overview.md) | 接口、与熔断器协作 |
| [均衡算法](loadbalancer/algorithms.md) | 轮询、随机、加权、一致性哈希（MD5）|

### 可靠性
| 文档 | 说明 |
|------|------|
| [熔断器](reliability/circuit-breaker.md) | 三状态机、SlidingWindow、服务端拦截器 |
| [限流器](reliability/rate-limiter.md) | 令牌桶（纳秒精度）、滑动窗口（时间戳数组）|
| [重试机制](reliability/retry.md) | 可重试错误码、重试间隔 |

### 可观测性
| 文档 | 说明 |
|------|------|
| [Prometheus 指标](observability/metrics.md) | 内置指标、标签、Grafana 面板建议 |

### 配置管理 `pkg/config`
| 文档 | 说明 |
|------|------|
| [配置管理](config/configuration.md) | YAML 结构、Builder 函数、环境变量覆盖 |

### Mini-RPC
| 文档 | 说明 |
|------|------|
| [Mini-RPC 概述](mini-rpc/overview.md) | 教学版精简实现与生产版对比 |

---

## 源码导航

```
RPCinGo/
├── pkg/
│   ├── protocol/       ← 消息格式、头部、错误码、Metadata
│   ├── codec/          ← JSON / Protobuf 编解码 + Gzip 压缩
│   ├── transport/      ← 传输接口
│   │   └── tcp/        ← TCP 客户端/服务端实现
│   ├── server/         ← RPC 服务端核心
│   ├── client/         ← RPC 客户端核心
│   ├── pool/           ← 连接池 + PoolManager
│   ├── registry/       ← 注册/发现接口
│   │   ├── etcd/       ← etcd v3 实现
│   │   └── memory/     ← 内存实现（测试用）
│   ├── loadbalancer/   ← 4 种负载均衡算法
│   ├── ratelimiter/    ← 令牌桶 + 滑动窗口
│   ├── circuitbreaker/ ← 熔断器三状态机
│   ├── interceptor/    ← 拦截器链（日志/监控/限流/熔断/恢复/重试）
│   └── config/         ← YAML 配置加载
├── mini-rpc/           ← 教学版精简实现（~1500行）
├── examples/
│   ├── calculator/     ← Protobuf 强类型示例
│   └── microservice/   ← 完整微服务示例
└── docs/               ← 其他文档
```
