# RPC-in-Go 使用指南

## 📚 文档导航

### 入门文档

1. **[架构总览](./00-architecture-overview.md)** 🏗️
   - 项目简介
   - 整体架构
   - 技术栈
   - 开发进度

### 分层文档

2. **[Protocol 层](./01-protocol-layer.md)** 📦
   - Header（协议头）
   - Request/Response（消息）
   - Error（错误）
   - Metadata（元数据）

3. **[Codec 层](./02-codec-layer.md)** 🔄
   - JSON 编解码
   - Protobuf 编解码
   - 流式编解码
   - 压缩支持

4. **[Transport 层](./03-transport-layer.md)** 🚀
   - TCP Client/Server
   - 连接池
   - 性能优化

### 设计文档

5. **[架构设计与规划](../design/ARCHITECTURE_PLAN.md)** 📋
   - 完整的 12 周计划
   - 技术选型
   - 实施路线图

---

## 🎯 快速索引

### 我想...

**学习框架原理**
→ 从 [架构总览](./00-architecture-overview.md) 开始

**了解消息格式**
→ 查看 [Protocol 层](./01-protocol-layer.md)

**了解序列化**
→ 查看 [Codec 层](./02-codec-layer.md)

**了解网络传输**
→ 查看 [Transport 层](./03-transport-layer.md)

**了解完整规划**
→ 查看 [架构设计](../design/ARCHITECTURE_PLAN.md)

---

## 📖 阅读顺序建议

### 新手入门

```
1. 架构总览 (了解全局)
   ↓
2. Protocol 层 (理解消息)
   ↓
3. Codec 层 (理解序列化)
   ↓
4. Transport 层 (理解传输)
```

### 深入学习

```
1. 架构设计文档 (理解设计思路)
   ↓
2. 各层详细文档 (API 参考)
   ↓
3. 源码阅读 (实现细节)
   ↓
4. 测试代码 (使用示例)
```

---

## 💡 学习建议

### 理论与实践结合

```
1. 先看文档（理解原理）
2. 再看代码（理解实现）
3. 写测试代码（动手实践）
4. 运行调试（加深理解）
```

### 循序渐进

```
Level 1: 使用框架
  - 了解 API
  - 运行示例
  - 简单应用

Level 2: 理解框架
  - 阅读文档
  - 理解设计
  - 源码阅读

Level 3: 扩展框架
  - 添加新功能
  - 性能优化
  - 贡献代码
```

---

## 🔗 相关资源

### Go 学习

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### RPC 和微服务

- [gRPC Documentation](https://grpc.io/docs/)
- [Microservices Patterns](https://microservices.io/)

### 分布式系统

- [MIT 6.824](https://pdos.csail.mit.edu/6.824/)
- [Designing Data-Intensive Applications](https://dataintensive.net/)

---

## 📝 文档版本

- **v1.0**: 2026-01-02 - 初始版本（Protocol + Codec + Transport）
- **v1.1**: 计划中 - 添加 RPC Client/Server
- **v1.2**: 计划中 - 添加服务注册发现

---

**维护者**: Kunhua Huang  
**最后更新**: 2026-01-02





