# Deepwiki 索引 — RPCinGo

## 摘要

| 项目 | 说明 |
|------|------|
| **目的** | Go 实现的生产级 RPC 框架，附教学版 mini-rpc |
| **技术栈** | Go 1.24.5 · TCP · JSON/Protobuf · etcd · Prometheus |
| **入口点** | `server.New()` / `client.New()` / `config.Load()` |
| **从哪开始** | [概述](overview.md) → [架构](architecture.md) → 对应[模块](#模块) |
| **导航方式** | 核心概念看架构和数据流；深入某模块看 modules/；操作指南看 guides/ |

## 从这里开始

- [概述](overview.md) — 项目目的、目录布局、入口点、常见陷阱
- [架构](architecture.md) — 五层架构、组件关系、接口契约、可靠性设计
- [数据流](data-flow.md) — 请求全链路 18 个阶段、错误传播、Metadata 流转

## 核心指南

- [依赖关系](dependencies.md) — go.mod 依赖、外部服务、版本约束
- [配置指南](guides/configuration.md) — 完整 YAML 配置参考、各组件配置详解
- [可观测性](guides/telemetry.md) — Prometheus 指标、PromQL 查询、结构化日志、分布式追踪
- [测试指南](guides/testing.md) — 单元/集成测试、测试文件分布、常见测试问题
- [架构深化重构记录（2026-06-02）](guides/deepening-refactors.md) — C1-C5 五项深化：connSource 缝、错误码单表、option 直通、协议死字段、哈希亲和度

## 模块

### 网络与传输

- [Protocol（协议层）](modules/protocol.md) — 20字节固定头、Request/Response、ErrorCode 枚举
- [Codec（编解码层）](modules/codec.md) — JSON/Protobuf/Gzip、Codec 与 StreamCodec 接口
- [Transport（传输层）](modules/transport.md) — TCP 客户端/服务端、两阶段帧读取、双信号量并发控制
- [Pool（连接池）](modules/pool.md) — ConnectionPool、PoolManager、PooledConnection、连接复用统计

### 服务端与客户端

- [Server（服务端）](modules/server.md) — 服务注册、反射分发、拦截器链、etcd 自动注册
- [Client（客户端）](modules/client.md) — Fixed/Discovery 双模式、Call/CallTyped、Watch 机制

### 服务发现与路由

- [Registry（注册中心）](modules/registry.md) — etcd 租约注册、Watch 实时推送、Memory 测试实现
- [LoadBalancer（负载均衡）](modules/loadbalancer.md) — Round Robin / Random / Weighted / Consistent Hash

### 可靠性与可观测性

- [CircuitBreaker（熔断器）](modules/circuitbreaker.md) — 三状态机、滑动窗口统计、per-address 独立熔断
- [RateLimiter（限流器）](modules/ratelimiter.md) — 令牌桶（纳秒精度）、滑动窗口、与熔断器的关系
- [Interceptor（拦截器）](modules/interceptor.md) — Recovery/Logging/Metrics/RateLimit/Retry/Tracing、Chain 组合
- [Tracing（分布式追踪）](modules/tracing.md) — OTel TracerProvider、Jaeger、W3C+B3 传播、metadataCarrier

### 配置与学习

- [Logger（日志）](modules/logger.md) — Logger 接口、slog 默认实现、Nop()、框架内集成点
- [Config（配置）](modules/config.md) — YAML 解析、BuildServerOptions/BuildClientOptions、字符串枚举映射
- [mini-rpc（教学版）](modules/mini-rpc.md) — ~1,500 行精简实现、异步 Go() 方法、与生产版对比

## 术语表

- [术语表](glossary.md) — 35+ 核心术语定义（RPC、熔断、令牌桶、一致性哈希等）

## 生成元数据

- [GENERATION.md](GENERATION.md) — 生成时间、commit hash、来源目录
