# 熔断器与限流器文档

## 概述

熔断器（Circuit Breaker）和限流器（Rate Limiter）是保护分布式系统的两个关键组件。

### 核心价值

```
Circuit Breaker:
  防止级联故障
  快速失败
  自动恢复

Rate Limiter:
  保护服务不过载
  流量整形
  防止雪崩
```

---

## Circuit Breaker (熔断器)

### 状态机

```
Closed (关闭) - 正常状态
  ↓ 失败率 > 50%
Open (打开) - 熔断状态
  ↓ 60 秒后
Half-Open (半开) - 试探状态
  ↓ 成功 2 次 / 失败 1 次
Closed / Open
```

### 配置参数

```go
Config{
    FailureThreshold: 0.5,   // 50% 失败率触发
    Timeout:          60s,   // Open 持续 60 秒
    SuccessThreshold: 2,     // 成功 2 次恢复
    MinRequests:      5,     // 最少 5 个请求才统计
    Interval:         10s,   // 统计窗口 10 秒
}
```

### 使用示例

```go
cb := circuitbreaker.New(config)

result, err := cb.Call(ctx, func() (interface{}, error) {
    return rpcClient.Call(...)
})

if err == circuitbreaker.ErrCircuitOpen {
    // Fallback
    return cachedData, nil
}
```

### 集成方式

**1. Client 端 (推荐)**:
```go
type Client struct {
    breakers map[service]*CircuitBreaker
}

// Per-service breaker
cb := client.getCircuitBreaker("UserService")
result, _ := cb.Call(ctx, rpcCall)
```

**2. Middleware**:
```go
srv.Use(
    CircuitBreakerInterceptor(cb),
)
```

---

## Rate Limiter (限流器)

### 算法对比

| 算法 | 性能 | 突发 | 精确度 | 推荐度 |
|------|------|------|--------|--------|
| Token Bucket | ⭐⭐⭐⭐⭐ | ✅ 允许 | 近似 | ⭐⭐⭐⭐⭐ |
| Sliding Window | ⭐⭐⭐⭐ | ❌ 严格 | 精确 | ⭐⭐⭐⭐ |

### Token Bucket 原理

```
Bucket (容量 100):
  ┌──────────┐
  │ ●●●●●●   │ 60 tokens
  └──────────┘
   ↑     ↓
生成 10/s 消费

特点:
- 允许突发 (burst = capacity)
- 平滑限流
- 高性能
```

### Sliding Window 原理

```
Window: 1 秒
Limit:  100 请求/秒

[─────────────────]
 09:59:59.5 → 10:00:00.5

Count requests in window
  < 100 → Allow
  >= 100 → Deny

特点:
- 精确限流
- 不允许突发
- 略慢
```

### 使用示例

```go
// Token Bucket
tb := ratelimiter.NewTokenBucketLimiter(1000, 2000)
if tb.Allow(ctx) {
    // Process request
}

// Wait mode
err := tb.Wait(ctx)  // Block until allowed

// Sliding Window
sw := ratelimiter.NewSlidingWindowLimiter(1000, time.Second)
if sw.Allow(ctx) {
    // Process
}
```

### Server 端集成

```go
srv := server.NewServer(...)

// Method 1: Middleware
rl := ratelimiter.NewTokenBucketLimiter(1000, 1000)
srv.Use(interceptor.RateLimit(rl))

// Method 2: Options (if implemented)
srv := server.NewServer(
    server.WithRateLimiter(1000),
)
```

---

## 性能测试结果

```
Circuit Breaker:
  Call (Closed):  < 100 ns/op
  State check:    < 10 ns/op

Rate Limiter:
  TokenBucket:    5-10 ns/op   (极快)
  SlidingWindow:  20-30 ns/op  (较快)
  
Concurrent:
  TokenBucket:    线程安全，无竞争
  SlidingWindow:  需要清理，有开销
```

---

## 最佳实践

### 1. 服务端限流

```go
// Token Bucket (推荐)
rate := 1000        // 1000 QPS
capacity := 2000    // 允许 2 倍突发

rl := NewTokenBucketLimiter(rate, capacity)

优点:
  正常: 1000 QPS 稳定
  突发: 2000 QPS 短时可应对
```

### 2. 客户端熔断

```go
// Per-service Circuit Breaker
breakers := map[service]*CircuitBreaker

config := &Config{
    FailureThreshold: 0.5,   // 50%
    Timeout:          60s,
    SuccessThreshold: 2,
}

用途:
  保护 Client 不被慢服务拖垮
  快速失败 + 降级
```

### 3. 降级策略

```go
result, err := cb.Call(ctx, rpcCall)

if err == circuitbreaker.ErrCircuitOpen {
    // 方案 1: 返回缓存
    return cache.Get(key)
    
    // 方案 2: 返回默认值
    return defaultValue
    
    // 方案 3: 调用备用服务
    return backupService.Call()
}
```

---

## 故障场景演练

### 场景 1: 服务过载

```
Problem:
  10,000 请求/秒 涌入
  → Server 只能处理 1,000/s
  → 崩溃

Solution:
  Rate Limiter (1000 QPS)
  → 拒绝超出部分
  → Server 稳定运行
```

### 场景 2: 下游故障

```
Problem:
  Service A 调用 Service B
  → Service B 故障（超时）
  → Service A 被拖垮

Solution:
  Circuit Breaker
  → 检测 B 故障
  → 熔断（不再调用 B）
  → A 快速失败 + 降级
```

### 场景 3: 突发流量

```
Problem:
  平时 100 QPS
  → 突然 1000 QPS（活动）
  → 需要应对

Solution:
  Token Bucket (rate=100, capacity=500)
  → 平时: 100 QPS
  → 突发: 可应对 500 QPS 短时
  → 持续高负载: 限制在 100 QPS
```

---

## 对比 Java 版本

| 特性 | Java | Go |
|------|------|-----|
| Circuit Breaker | ❌ 无 | ✅ 完整 |
| Rate Limiter | ❌ 无 | ✅ 2种算法 |
| Error Mapping | 基础 | 完善 |

Go 版本新增功能！

---

**文档版本**: v1.0  
**最后更新**: 2026-01-03  
**作者**: Kunhua Huang




