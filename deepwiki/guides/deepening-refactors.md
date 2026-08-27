# 架构深化重构记录（2026-06-02）

本文记录一轮以**深化模块**（deepening）为目标的重构：把接口几乎和实现一样复杂的**浅模块**，改造成「小接口、大行为」的**深模块**，从而提升可测性与定位性（locality）。术语遵循架构评审词表：模块 / 接口 / 实现 / 深度 / 缝（seam）/ 适配器 / 杠杆 / 定位性。

五项重构互相关联，按依赖顺序落地。每项都附 `file:line` 与对应测试。

| # | 重构 | 强度 | 关键文件 |
|---|------|------|---------|
| C1 | 用 `connSource` 缝收拢客户端 fixed/discovery 双路径 | Strong | `pkg/client/source.go`、`pkg/client/client.go` |
| C2 | 错误码映射收敛为单一声明式表 | Strong | `pkg/server/error_map.go` |
| C3 | 打通 server / transport 两套 option 面 | Worth exploring | `pkg/server/options.go`、`pkg/server/server.go` |
| C4 | 不再让协议层暴露被传输层丢弃的死字段 | Worth exploring | `pkg/protocol/request.go`、`pkg/transport/tcp/codec.go` |
| C5 | 让一致性哈希亲和度可达 | Speculative | `pkg/loadbalancer/balancer.go`、`pkg/client/affinity.go` |

---

## C1 — `connSource` 缝：收拢 fixed / discovery 双路径

### 问题
`Client.Call` 用 `c.fixedMode` 布尔在 `callFixed` 与 `callWithDiscovery` 间分叉，两条路径各自取连接、各自重复一段 `resp.IsError() → unmapError` 处理。调用方还得知道：用哪个构造器、哪些 option 在哪个模式下生效（fixed 模式静默忽略 `WithDiscovery/WithLoadBalancer/WithCircuitBreaker`，discovery 模式硬编码池大小 100/10，`WithPoolSize` 是 no-op）。接口几乎和实现一样复杂 —— 典型浅模块。

### 改动
抽出 `connSource` 接口（缝）：

```go
type connSource interface {
    acquire(ctx context.Context, req *protocol.Request) (*pool.PooledConnection, error)
    Close() error
}
```

两个适配器满足它（两个适配器 = 真实的缝）：
- `fixedSource` —— 单地址单池（`pkg/client/source.go`）。
- `discoverySource` —— discovery + 负载均衡 + per-endpoint 池管理器，并独占实例缓存与 watch 协程（`pkg/client/source.go`）。

`Call` 不再分叉，统一走 `callOnce`（`pkg/client/client.go`）。熔断器仍由 `Client` 在 `breakerOn` 时包裹 `callOnce`（韧性关注点，不属于取连接）。

### 顺带修掉的「静默」
- `NewClient` 传入 `WithDiscovery` 现在**报错**，而非静默忽略（`pkg/client/client.go`）。
- `NewDiscoveryClient` 通过 `pool.WithManagerPoolSize` 把 `WithPoolSize` 真正传给池管理器（`pkg/pool/pool_manager.go`），不再硬编码 100/10。

### 收益
- 杠杆：`Call` 一条路径，不再两条。
- 定位性：取连接逻辑集中在缝后；discovery 全部逻辑集中在 `discoverySource`。
- 可测性：测试只面对一个接口，可注入假的 balancer / source。
- 删除：重复的错误处理块、`fixedMode` 分支。

### 测试
`pkg/client/source_test.go`：`TestPickUsesHashKeyWhenPresent`、`TestPickFallsBackWithoutHashKey`、`TestNewClientRejectsDiscoveryOptions`。

---

## C2 — 错误码映射的单一声明式表

### 问题
`mapError`（Go 错 → 协议码）与 `unmapError`（协议码 → Go 错）是两段必须手工保持同步的 switch；覆盖已经漂移（encode 5 个、decode 6 个，`InvalidArgument` 只有 decode 没有 encode）。加一个码要改 ≥3 处。

### 改动
单一事实源 `errorTable []errorMapping{ code, sentinel, message }`（`pkg/server/error_map.go`）。两个方向都从它派生：
- `mapError` 用 `errors.Is` 命中 sentinel 取 code 与 message。
- `unmapError` 用 `sentinelForCode` 反查 sentinel，用 `%w` 包裹服务端消息返回 —— `errors.Is` 仍可命中（e2e 的 `errors.Is(err, ratelimiter.ErrRateLimitExceeded)` 保持有效）。

补齐了 `ErrInvalidArgument` sentinel，闭合 encode/decode 不对称。

### 收益
- 定位性：加一个码只改一处。
- 构造即对称：无法只改一个方向而忘了另一个。

### 测试
`pkg/server/error_map_test.go`：`TestErrorTableRoundTrip`（逐条往返）、`TestMapErrorUnknownIsInternal`、`TestUnmapErrorSuccessIsNil`、`TestUnmapErrorUnknownCode`。

---

## C3 — 打通 server / transport 两套 option 面

### 问题
`server.Option` 与 `transport.ServerOption` 是两套平行面，靠 `NewServer` 里一段手写翻译粘合，只接了 6 个 transport 选项中的 3 个。`HandlerTimeout`（服务端唯一真实的 per-request 预算）、buffer、max body 从 `server.NewServer` **够不到**。

### 改动（`pkg/server/options.go`、`pkg/server/server.go`）
- 新增 `server.WithHandlerTimeout(d)`，补齐长期缺口。
- 新增 `server.WithTransportOptions(...transport.ServerOption)` 直通，让**每个** transport 旋钮都可达，且无需逐个写包装；直通项在翻译项之后应用，故优先级更高。
- `tcp.Server` 增加 `Options()` 访问器，便于上层校验哪些旋钮生效（也是可观测性改进）。

### 收益
- 杠杆：每个 transport 旋钮经单一构造器可达。
- 定位性：直通后无需维护翻译同步；新增 transport 选项无第三处编辑点。

### 测试
`pkg/server/options_test.go`：`TestHandlerTimeoutReachesTransport`、`TestTransportOptionsPassThrough`、`TestTransportOptionsTakePrecedence`。

### 备注
`config.BuildServerOptions` 现在可以接 `WithHandlerTimeout`（后续可补上 YAML 映射；本轮未动 config）。

---

## C4 — 协议层不再暴露被传输层丢弃的死字段

### 问题
两个「接口在说谎」的字段：
1. `protocol.NewRequest` 用全局原子计数器赋 `Request.ID`，随后 `tcp.Client.SendRequest` 用 per-connection 计数器**覆写** —— 协议层的 ID 是死的，发送前打 `req.ID` 会误导。
2. `Header.Codec` 字节写到线上，但接收端解码时从不读它（codec 在连接上静态）—— 死元数据。

### 改动
1. **ID 归属传输层**（`pkg/protocol/request.go`）：删掉全局计数器，`NewRequest` 不再赋 ID（保持 0）；多路复用 key 由 `tcp.Client.SendRequest` 这唯一需要它的地方拥有。`Request.ID` 字段保留（仍要上线），但文档明确「发送前为 0」。
2. **honour `Header.Codec`**（`pkg/transport/tcp/codec.go`、`client.go`）：新增 `DecodeRequestWith/DecodeResponseWith(codecType, data)`，`ReadRequest/ReadResponse` 按 `header.Codec` 解码；客户端 `pendingCall` 记下 `respCodec`，`SendRequest` 用它解码。这个字节从此**有意义**（编码侧仍静态，解码侧按帧 codec）。

### 契约变更
`pkg/protocol/message_test.go` 原来断言「全局 ID 单调递增」。该断言正是本次有意移除的行为，已改写为 `TestNewRequestDoesNotAssignID`（两个新建请求 ID 均为 0），并说明 per-connection 唯一性与响应路由由传输层覆盖（`pkg/transport/tcp/client_test.go` 已有）。

### 收益
- 接口不再说谎；定位性：多路复用 key 只活在 demux 处。
- honour codec 字节 = 真正的 codec 深度（接收端可按帧 codec 解）。

### 测试
`pkg/transport/tcp/codec_headercodec_test.go`：`TestReadRequestHonoursHeaderCodec`（JSON 帧被静态 Protobuf 的接收端按 header 正确解出）。

---

## C5 — 让一致性哈希亲和度可达

### 问题
`ConsistentHash` 实现了按 key 的亲和度，但 `PickOptions.key` **未导出**，外部无法设置；无 key 时回退 `time.Now()` —— 一致性哈希静默退化为随机。实现比可达接口更丰富 —— 浪费的杠杆。

### 改动
- 导出可达入口（`pkg/loadbalancer/balancer.go`）：`NewPickOptions(key)`、`(*PickOptions).Key()`、`(*PickOptions).WithKey(key)`。
- 调用侧贯通（`pkg/client/affinity.go`、`source.go`、`client.go`）：`client.WithHashKey(ctx, key)` 把 key 放进 ctx；`Call` 写入 `req.Metadata[MetaKeyHashKey]`；`discoverySource.pick` 在 balancer 实现 `BalancerWithOptions` 且 key 存在时调 `PickWithOptions`，否则回退 `Pick`。key 自然搭在 C1 的 `ConnectionSource` 缝上。

### 收益
- 杠杆：被搁置的行为变可达；一致性哈希不再退化为随机。
- 可测性：可断言「同 key → 同实例」。

### 测试
`pkg/loadbalancer/affinity_test.go`：`TestConsistentHashAffinityReachable`、`TestPickOptionsKeyAccessor`。客户端贯通见 `pkg/client/source_test.go`。

---

## 用法速查

```go
// C3：服务端设置 handler 预算 + 任意 transport 旋钮
srv := server.NewServer(
    server.WithHandlerTimeout(2*time.Second),
    server.WithTransportOptions(transport.WithMaxRequestBodySize(8<<20)),
)

// C5：发现模式下按租户做亲和路由
cli, _ := client.NewDiscoveryClient(
    client.WithDiscovery(disc),
    client.WithLoadBalancer(loadbalancer.NewConsistentHash()),
    client.WithPoolSize(500, 50), // C1：发现模式下不再是 no-op
)
ctx := client.WithHashKey(context.Background(), "tenant-7")
resp, _ := cli.Call(ctx, "Svc", "M", args)
```
