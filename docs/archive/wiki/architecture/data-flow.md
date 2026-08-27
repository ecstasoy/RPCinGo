# 数据流

## Fixed 模式完整请求链路

```
应用代码
    │
    │  cli.Call(ctx, "UserService", "GetUser", args)
    ▼
─────────────────────────── pkg/client ─────────────────────────────
    │
    │  1. 构建 Request：
    │     ID      = atomic.AddUint64(&globalID, 1)  // 全局原子自增
    │     Service = "UserService"
    │     Method  = "GetUser"
    │     Args    = args                              // 原始 Go 对象
    │     Timeout = callTimeout.Milliseconds()
    │     CreatedAt = time.Now().UnixMilli()
    │     Metadata = ctx 中的 metadata（如 trace-id）
    │
    │  2. 执行客户端拦截器链（前置）
    │     → Retry → Logging → ...
    │
    │  3. pool.Get(ctx)  // 从连接池获取 TCP 连接
    │
─────────────────────────── pkg/codec ──────────────────────────────
    │
    │  4. codec.Encode(request)  // 序列化 Request 结构体
    │     JSON: json.Marshal(request)  → []byte（如 "{"id":42,...}"）
    │
    │  5. compressor.Compress(body)  // 可选 Gzip 压缩
    │
─────────────────────────── pkg/transport/tcp ──────────────────────
    │
    │  6. 构建 Header（20 字节）：
    │     [0:2]   Magic = 0xCAFE
    │     [2]     Version = 1
    │     [3]     MsgType = 1（Request）
    │     [4]     Codec = 1（JSON）
    │     [5]     Compress = 0（None）
    │     [6:8]   Reserved = 0x0000
    │     [8:16]  RequestID = 42（big-endian）
    │     [16:20] BodyLength = 156（big-endian）
    │
    │  7. conn.Write([Header(20B) | Body(156B)])
    │     TCP 无延迟（NoDelay=true），立即发送
    │
    │═══════════════ 网络传输 ═══════════════════
    │
─────────────────────────── pkg/transport/tcp (服务端) ─────────────
    │
    │  8. io.ReadFull(conn, headerBuf[:20])   // 精确读取 20 字节 Header
    │  9. decodeHeader(headerBuf)             // 验证 Magic=0xCAFE，获取 BodyLength=156
    │
    │  10. io.ReadFull(conn, bodyBuf[:156])   // 按 BodyLength 读取 Body
    │
    │  11. compressor.Decompress(bodyBuf)    // 解压（若 Header.Compress != None）
    │
─────────────────────────── pkg/codec ──────────────────────────────
    │
    │  12. codec.Decode(bodyBuf, &request)   // 反序列化 Body → Request 对象
    │
─────────────────────────── pkg/server ─────────────────────────────
    │
    │  13. 执行服务端拦截器链（前置 → 后置）：
    │      Recovery → Logging → Metrics → RateLimit → [Handler] → RateLimit → Metrics → Logging → Recovery
    │
    │  14. ServiceRegistry.Invoke(ctx, request)：
    │      key = "UserService.GetUser"
    │      handler = registry.services["UserService"].methods["GetUser"].handler
    │
    │  15. 反序列化 Args → *UserRequest（根据 ArgsCodec 选择 JSON/Protobuf）
    │
    │  16. handler(ctx, *UserRequest)  // 反射调用
    │      → return (*UserResponse, nil)
    │
    │  17. 构建 Response：
    │      ID        = request.ID（= 42，用于匹配）
    │      Data      = *UserResponse
    │      Error     = nil
    │      ServerTime = time.Now().UnixMilli()
    │
─────────────────────────── pkg/codec ──────────────────────────────
    │
    │  18. codec.Encode(response) → []byte
    │  19. compressor.Compress(body) // 可选
    │
─────────────────────────── pkg/transport/tcp ──────────────────────
    │
    │  20. 构建 Response Header（20 字节，MsgType=2）
    │  21. conn.Write([Header | Body])
    │
    │═══════════════ 网络传输 ═══════════════════
    │
─────────────────────────── pkg/client ─────────────────────────────
    │
    │  22. ReadResponse()：ReadFull(Header) + ReadFull(Body)
    │  23. codec.Decode(body, &response)
    │  24. unmapError(response.Error) → Go error（client/error_map.go）
    │  25. pool.Put(conn)          // 归还连接
    │  26. 执行客户端拦截器链（后置）
    │
    ▼
应用代码接收 (result, error)
```

## Discovery 模式附加步骤

在步骤 3（pool.Get）之前，Discovery 模式额外执行：

```
步骤 3 之前：

    3a. c.instancesMu.RLock()
        instances = c.instances（本地缓存，由 Watch goroutine 维护）
        c.instancesMu.RUnlock()

    3b. loadBalancer.Pick(instances)
        → 选中 instance{Address:"10.0.0.1", Port:8080}

    3c. breaker = c.getBreaker("10.0.0.1:8080")
        if !breaker.Allow() {
            return nil, ErrServiceUnavailable  // 熔断，直接返回
        }

    3d. pool = c.poolManager.GetPool("10.0.0.1:8080")
        conn = pool.Get(ctx)

步骤 25 之后：

    25a. if err != nil {
             breaker.RecordFailure()  // 失败计数
         } else {
             breaker.RecordSuccess()  // 成功计数
         }
```

## 后台 Watch goroutine（Discovery 模式）

与请求链路并行运行的后台 goroutine：

```
NewDiscoveryClient()
    │
    └── go watchInstances():
            watcher, _ := discovery.Watch(ctx, serviceName)
            for {
                event := watcher.Next()  // 阻塞等待 etcd 事件
                switch event.Type {
                case EventAdd:
                    instancesMu.Lock()
                    instances = append(instances, event.Instance)
                    instancesMu.Unlock()

                case EventDelete:
                    instancesMu.Lock()
                    instances = removeByID(instances, event.Instance.ID)
                    instancesMu.Unlock()
                    poolManager.RemovePool(event.Instance.Address)  // 清理连接池

                case EventUpdate:
                    instancesMu.Lock()
                    updateInSlice(instances, event.Instance)
                    instancesMu.Unlock()
                }
            }
```

## 错误传播路径

```
Handler 返回错误
    │
    │ error_map.go (server)
    ▼
protocol.Error{Code: X, Message: "..."}
    │ 写入 Response.Error 字段
    │ 序列化 + 网络传输
    ▼
protocol.Error{Code: X, Message: "..."}
    │ error_map.go (client)
    ▼
Go error（context.DeadlineExceeded / ErrServiceUnavailable / 等）
    │
    ▼
CircuitBreaker.RecordFailure()  // 失败计数驱动熔断
```

## 关键数据类型流转

```
应用层对象（Go struct / proto.Message）
    │ json.Marshal / proto.Marshal
    ▼
[]byte（序列化 payload）
    │ 可选：gzip.Compress
    ▼
[]byte（压缩后 payload）
    │ 加 20 字节 Header
    ▼
[]byte = [Header(20B) | Body(N B)]  ← TCP 实际传输内容
    │ io.ReadFull × 2（先 Header，再 Body）
    ▼
[]byte（压缩后 payload）
    │ 可选：gzip.Decompress
    ▼
[]byte（序列化 payload）
    │ json.Unmarshal / proto.Unmarshal
    ▼
应用层对象（Go struct / proto.Message）
```

## 相关文档

- [协议头](../protocol/header.md) — Header 字段详解
- [TCP 传输](../transport/tcp.md) — 两阶段读取实现
- [Codec 概述](../codec/overview.md) — 序列化流程
- [拦截器链](../server/interceptors.md) — 拦截器执行顺序
- [熔断器](../reliability/circuit-breaker.md) — Allow/Record 方法
