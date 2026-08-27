# 模块：RateLimiter（限流器）

## 职责

- 定义 `RateLimiter` 接口（`Allow()`, `AllowN()`, `Wait()`）
- 提供**令牌桶（TokenBucket）**：惰性填充，纳秒精度，支持突发，无后台 goroutine
- 提供**滑动窗口（SlidingWindow）**：无边界效应，严格均匀限流
- 通过 `RateLimit` 拦截器集成到服务端请求处理链

**源码位置**：`pkg/ratelimiter/`（limiter.go、token_bucket.go 130 行、sliding_window.go 75 行）

## 关键文件

| 文件 | 行数 | 职责 |
|------|------|------|
| `pkg/ratelimiter/limiter.go` | — | `RateLimiter` 接口定义、错误变量 |
| `pkg/ratelimiter/token_bucket.go` | 130 | `TokenBucketLimiter` 实现 |
| `pkg/ratelimiter/sliding_window.go` | 75 | `SlidingWindowLimiter` 实现 |

---

## 接口定义

```go
// pkg/ratelimiter/limiter.go
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

---

## 令牌桶（TokenBucketLimiter）

**源码**：`pkg/ratelimiter/token_bucket.go`（130 行）

### 核心结构

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
```

### 关键实现：惰性补充（Lazy Refill）

不使用 `time.Ticker` 或后台 goroutine，每次 `Allow()` 时按时间差惰性补充令牌：

```go
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
// rate=1000 req/s，burst=100（允许瞬时 100 个请求突发）
limiter := ratelimiter.NewTokenBucket(1000, 100)

// 非阻塞
if !limiter.Allow() {
    return nil, ratelimiter.ErrRateLimitExceeded
}

// 阻塞（适合批量任务，等待令牌可用）
if err := limiter.Wait(ctx); err != nil {
    return nil, err // ctx 超时
}
```

---

## 滑动窗口（SlidingWindowLimiter）

**源码**：`pkg/ratelimiter/sliding_window.go`（75 行）

### 实现：时间戳数组

```go
type SlidingWindowLimiter struct {
    mu         sync.Mutex
    limit      int           // 窗口内最大请求数
    window     time.Duration // 窗口大小
    timestamps []time.Time   // 请求时间戳数组（有序）
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

    if len(l.timestamps)+n > l.limit {
        return false
    }

    for j := 0; j < n; j++ {
        l.timestamps = append(l.timestamps, now)
    }
    return true
}
```

### 无边界效应

与固定窗口（每秒清零）不同，滑动窗口在任意时刻的窗口内请求数都不超过 `limit`，没有固定窗口在边界处允许 2×limit 请求的问题。

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
| CPU | O(1)（惰性补充）| O(k)（清理过期条目）|
| 后台 goroutine | 无 | 无 |
| 适合场景 | API 网关、有突发流量 | 严格限流、数据库写保护 |

---

## 集成拦截器

```go
limiter := ratelimiter.NewTokenBucket(10000, 500)

srv := server.NewServer(
    server.WithInterceptors(
        interceptor.NewRateLimitInterceptor(limiter),
    ),
)
```

超限时拦截器返回 `ErrRateLimitExceeded`，服务端 `error_map.go` 映射为 `ResourceExhausted(8)` 错误码。

---

## 按方法独立限流

```go
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

---

## 与熔断器的关系

```
RateLimit 拦截器（先执行）
    → 超出 QPS 直接返回 ResourceExhausted（HTTP 429 等价）
    → 请求进入 CircuitBreaker 检查（熔断）
```

限流保护**整体流量上限**，熔断保护**下游故障传播**，二者互补。

---

## 设计模式

| 模式 | 体现 |
|------|------|
| 惰性填充（Lazy Refill） | 不用 timer，每次 Allow() 时按时间差补充令牌 |
| 策略模式 | RateLimiter 接口支持任意算法实现 |

## 测试

| 测试文件 | 内容 |
|---------|------|
| `pkg/ratelimiter/limiter_test.go` | 令牌桶速率、突发上限、滑动窗口均匀性 |

## Source References

- `pkg/ratelimiter/limiter.go`
- `pkg/ratelimiter/token_bucket.go`（130 行）
- `pkg/ratelimiter/sliding_window.go`（75 行）
- `pkg/ratelimiter/limiter_test.go`
- `pkg/interceptor/ratelimit.go`（20 行）
- `wiki/reliability/rate-limiter.md`
- `wiki/reliability/retry.md`
