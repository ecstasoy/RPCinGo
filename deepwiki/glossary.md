# 术语表

## 核心术语

- **RPC（Remote Procedure Call）**：远程过程调用。让调用远端服务的方法像调用本地函数一样简单，屏蔽网络通信细节。

- **Fixed 模式**：客户端直连固定地址的 RPC 调用模式，适合单节点开发/测试场景，无需服务发现。

- **Discovery 模式**：客户端通过服务注册中心（如 etcd）动态获取服务实例地址，配合负载均衡使用的 RPC 调用模式。

- **Protocol Header（协议头）**：RPCinGo 每帧最开始的 20 字节固定格式，包含 Magic（0xCAFE）、Version、Codec 类型、压缩类型、RequestID、BodyLength 等字段，用于帧边界识别和路由。

- **Magic Number（魔数）**：协议头中的固定标识 `0xCAFE`，接收方通过校验此值判断帧是否合法，防止协议错位。

- **RequestID**：协议头中的 8 字节唯一请求标识，客户端用于将异步响应与发出的请求匹配（存储在内存 Map 中）。

- **Codec（编解码器）**：负责将 Go 对象序列化为字节（Encode）和将字节反序列化为 Go 对象（Decode）的组件。RPCinGo 支持 JSON 和 Protobuf。

- **StreamCodec（流编解码器）**：带长度前缀帧的编解码接口，Protobuf 使用 4 字节大端长度前缀，适合流式读写场景。

- **Compressor（压缩器）**：对已编码字节进行压缩/解压缩的组件。Gzip 压缩可减少网络带宽，但增加 CPU 开销。

- **CompressedCodec**：将 Codec 和 Compressor 组合的装饰器，透明地先编码再压缩（或先解压再解码）。

- **ServiceInstance（服务实例）**：注册中心中的服务节点描述，包含 ID、服务名、版本、地址、端口、权重、状态、Metadata。

- **ServiceEvent（服务事件）**：服务实例变更通知，类型为 Add/Update/Delete，通过 Watch channel 推送给客户端。

- **Registry（注册中心）**：服务注册与注销的接口，提供 Register/Deregister/ListServices 方法。

- **Discovery（服务发现）**：获取服务实例列表和监听变更的接口，提供 GetInstances/Watch 方法。

- **LoadBalancer（负载均衡器）**：从多个服务实例中选择一个目标的策略组件，Pick() 方法接受实例列表，返回选中实例。

- **Round Robin（轮询）**：按顺序依次选择实例的负载均衡算法，使用原子计数器实现无锁轮转。

- **Weighted Round Robin（加权轮询）**：按 ServiceInstance.Weight 字段比例分配流量的负载均衡算法。

- **Consistent Hash（一致性哈希）**：使用 MD5 + 虚拟节点环的负载均衡算法，相同 Key 路由到相同实例，适合有状态服务。

- **CircuitBreaker（熔断器）**：监控请求失败率，在下游故障时自动中断请求（Open 状态），防止级联雪崩，并在一定时间后自动探测恢复（HalfOpen → Closed）。

- **Sliding Window（滑动窗口）**：用于统计近期时间范围内的请求成功/失败次数的数据结构，由多个时间桶组成，旧桶数据自动淘汰。

- **TokenBucket（令牌桶）**：限流算法，以固定速率填充令牌，请求消耗令牌，桶空则限流。支持突发（burst）流量吸收。

- **RateLimiter（限流器）**：控制请求速率的组件，RPCinGo 提供令牌桶和滑动窗口两种实现。

- **Interceptor（拦截器/中间件）**：包裹 RPC 处理器执行的函数，可在请求前后插入横切逻辑（日志、限流、监控等）。

- **Interceptor Chain（拦截器链）**：多个拦截器按顺序组合后的执行链，外层拦截器先执行前处理，后执行后处理。

- **Recovery（恢复拦截器）**：捕获处理器中 panic 并将其转换为 Internal 错误码的拦截器，防止进程崩溃。

- **PooledConnection（池化连接）**：连接池返回的连接包装类，调用 Close() 将连接归还池而非真正关闭。

- **PoolManager（连接池管理器）**：Discovery 模式下管理多个地址对应连接池的组件，使用双重检查锁定延迟创建。

- **Lease（租约）**：etcd 提供的 TTL 机制，服务注册时关联租约，定期 KeepAlive 续约；服务宕机后 TTL 到期自动删除注册记录。

- **Metadata**：随 RPC 请求透明传递的 key-value map，用于传递 trace-id、认证 token、区域信息等横切关注点。

- **Options Pattern（选项模式）**：通过 `WithXxx()` 函数返回 Option 闭包，灵活配置结构体的 Go 设计模式，避免大量构造参数。

- **Reflection（反射）**：RPCinGo 服务端通过 `reflect` 包自动发现并注册服务对象的公开方法，实现动态路由。

- **ProtocolCodec**：Transport 层中负责两阶段帧读写的组件，先读 20B Header，再按 BodyLength 读 Body。

- **ErrorCode（错误码）**：RPCinGo 定义的 11 个标准错误码（OK、Canceled、Unknown、InvalidArgument、DeadlineExceeded 等），比字符串错误更易于客户端程序化处理。

- **mini-rpc**：RPCinGo 仓库中的教学精简版本（~1,500 行），仅含核心 RPC 原理，无生产级功能（无服务发现、熔断、连接池等）。

- **CallTyped**：客户端提供的强类型调用方法，参数和响应必须实现 `proto.Message`，与 `Call()` 相比提供编译期类型安全。

- **SemConns / SemReqs**：服务端 TCPServer 中的两个信号量，分别限制最大并发连接数和最大并发请求数。

## Source References

- `pkg/protocol/`
- `pkg/codec/`
- `pkg/transport/tcp/`
- `pkg/server/`
- `pkg/client/`
- `pkg/pool/`
- `pkg/registry/`
- `pkg/loadbalancer/`
- `pkg/circuitbreaker/`
- `pkg/ratelimiter/`
- `pkg/interceptor/`
- `wiki/architecture/design-patterns.md`
