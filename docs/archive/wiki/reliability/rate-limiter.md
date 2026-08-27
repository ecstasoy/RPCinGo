# 限流器

## 概述

限流器控制服务端接受请求的速率，防止流量突增压垮服务。提供两种算法：**令牌桶**（支持突发）和**滑动窗口**（严格均匀）。

**源码位置**：`pkg/ratelimiter/`（limiter.go、token_bucket.go 130行、sliding_window.go 75行）

## 接口定义

**源码**：`pkg/ratelimiter/limiter.go`

```go
type RateLimiter interface {
    Allow() bool                    // 非阻塞，立即返回是否允许 1 个请求
    AllowN(n int) bool              // 非阻塞，是否允许 n 个请求
    Wait(ctx context.Context) error // 阻塞直到允许通过或 ctx 超时
    Name() string
}

var (
    ErrRateLimitExceeded = errors.New("rate limit exceeded")
    ErrInvalidRequest    = errors.New("invalid request: n must be positive")
)
```

## 令牌桶（TokenBucketLimiter）

**源码**：`pkg/ratelimiter/token_bucket.go`（130 行）

### 工作原理

```
令牌桶（容量 = Burst）
    │
    ├── 以固定速率 Rate（个/秒）向桶中填充令牌
    ├── 桶满时停止填充（最多积累 Burst 个令牌）
    ├── 每个请求消耗 1 个令牌
    └── 桶为空时拒绝/阻塞请求
```

令牌桶允许**短时突发**（一次消耗多个令牌），长期速率受 Rate 约束。

### 实现细节：纳秒精度令牌补充

框架使用**惰性补充（Lazy Refill）**策略，不用定时器，而是在每次 `Allow()` 调用时计算应补充的令牌数：

```go
type TokenBucketLimiter struct {
    mu       sync.Mutex
    rate     float64   // 每秒补充的令牌数
    capacity float64   // 桶容量（= Burst）
    tokens   float64   // 当前令牌数
    last     time.Time // 上次更新时间
    // 纳秒余量跟踪，避免浮点精度丢失
    remainderNs int64
}

func (l *TokenBucketLimiter) Allow() bool {
    return l.AllowN(1)
}

func (l *TokenBucketLimiter) AllowN(n int) bool {
    if n <= 0 {
        return false
    }
    l.mu.Lock()
    defer l.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(l.last)

    // 计算应补充的令牌数（纳秒精度）
    elapsedNs := elapsed.Nanoseconds() + l.remainderNs
    newTokens := float64(elapsedNs) * l.rate / 1e9
    l.remainderNs = elapsedNs - int64(newTokens/l.rate*1e9) // 保留余量

    l.tokens = math.Min(l.tokens+newTokens, l.capacity)
    l.last = now

    if float64(n) > l.tokens {
        return false
    }
    l.tokens -= float64(n)
    return true
}
```

**纳秒余量的作用**：假设 rate=1000/s，每次调用间隔 0.5ms，理论应补充 0.5 个令牌。浮点累加会有精度丢失，通过保留整数纳秒余量（`remainderNs`），确保长期补充速率精确等于 rate。

### Wait（阻塞等待）

```go
func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
    for {
        if l.Allow() {
            return nil
        }
        // 计算需要等待的时间
        waitDuration := l.nextTokenDuration()
        select {
        case <-time.After(waitDuration):
            // 继续尝试
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 创建与使用

```go
// rate=1000 req/s，burst=100（允许瞬时 100 个请求）
limiter := ratelimiter.NewTokenBucket(1000, 100)

// 使用（非阻塞）
if !limiter.Allow() {
    return nil, ratelimiter.ErrRateLimitExceeded
}

// 使用（阻塞，适合批量任务）
if err := limiter.Wait(ctx); err != nil {
    return nil, err // ctx 超时
}
```

---

## 滑动窗口（SlidingWindowLimiter）

**源码**：`pkg/ratelimiter/sliding_window.go`（75 行）

### 工作原理

维护一个请求时间戳数组，每次 `Allow()` 时清除窗口外的旧时间戳，统计窗口内的请求数：

```go
type SlidingWindowLimiter struct {
    mu         sync.Mutex
    limit      int           // 窗口内最大请求数
    window     time.Duration // 窗口大小
    timestamps []time.Time   // 请求时间戳数组（有序）
}

func (l *SlidingWindowLimiter) Allow() bool {
    return l.AllowN(1)
}

func (l *SlidingWindowLimiter) AllowN(n int) bool {
    l.mu.Lock()
    defer l.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-l.window) // 窗口起始时间

    // 清除窗口外的旧时间戳（O(k)，k 为过期请求数）
    i := 0
    for i < len(l.timestamps) && l.timestamps[i].Before(cutoff) {
        i++
    }
    l.timestamps = l.timestamps[i:]

    // 检查当前窗口内的请求数
    if len(l.timestamps)+n > l.limit {
        return false
    }

    // 记录新请求时间戳
    for j := 0; j < n; j++ {
        l.timestamps = append(l.timestamps, now)
    }
    return true
}
```

### 无边界效应

与固定窗口算法（每秒清零）不同，滑动窗口在任意时刻的窗口内请求数都不超过 `limit`，没有固定窗口在边界处允许 2×limit 请求的问题。

### 创建与使用

```go
// 任意 1 秒内最多 1000 个请求
limiter := ratelimiter.NewSlidingWindow(1000, time.Second)

if !limiter.Allow() {
    return nil, ratelimiter.ErrRateLimitExceeded
}
```

---

## 两种算法对比

| 特性 | 令牌桶 | 滑动窗口 |
|------|--------|---------|
| 突发支持 | ✅（最多 Burst 个）| ❌（严格均匀）|
| 精确度 | 高（纳秒精度）| 高（无边界效应）|
| 内存 | O(1) | O(window × rate)（时间戳数组）|
| CPU | O(1) | O(k)（清理过期条目）|
| 适合场景 | API 网关、有突发的流量 | 严格限流、SLA 保障 |

## 集成拦截器

```go
limiter := ratelimiter.NewTokenBucket(10000, 500)

srv := server.NewServer(
    server.WithInterceptors(
        interceptor.NewRateLimitInterceptor(limiter),
    ),
)
```

超限时拦截器返回 `ErrRateLimitExceeded`，服务端 `error_map.go` 将其映射为 `ResourceExhausted` 错误码（HTTP 429 等价）。

## 按方法独立限流

```go
type MethodRateLimiter struct {
    limiters map[string]ratelimiter.RateLimiter
    defaults ratelimiter.RateLimiter
}

func NewMethodRateLimiter() interceptor.Interceptor {
    limiters := map[string]ratelimiter.RateLimiter{
        "UserService.GetUser":   ratelimiter.NewTokenBucket(5000, 200),
        "UserService.ListUsers": ratelimiter.NewTokenBucket(500, 20),  // 重查询严格限制
    }
    global := ratelimiter.NewTokenBucket(20000, 1000)

    return func(ctx context.Context, req *protocol.Request,
        next interceptor.Invoker) (interface{}, error) {

        key := req.Service + "." + req.Method
        l, ok := limiters[key]
        if !ok {
            l = global
        }
        if !l.Allow() {
            return nil, ratelimiter.ErrRateLimitExceeded
        }
        return next(ctx, req)
    }
}
```

## 相关文档

- [熔断器](circuit-breaker.md)
- [拦截器链](../server/interceptors.md) — RateLimit 拦截器
- [错误码](../protocol/error-codes.md) — ResourceExhausted (8)
