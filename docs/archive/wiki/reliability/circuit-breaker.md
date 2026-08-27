# 熔断器

## 概述

熔断器（Circuit Breaker）防止级联故障：当某服务实例持续出错，熔断器"跳闸"拒绝后续请求，给实例恢复时间，同时保护调用方不被拖垮。

**源码位置**：`pkg/circuitbreaker/`（breaker.go 197行、state.go 27行、window.go 125行）

## 三状态机

**源码**：`pkg/circuitbreaker/state.go`

```go
type State int

const (
    StateClosed   State = iota // 0：正常，请求正常通过
    StateOpen                  // 1：熔断，拒绝所有请求
    StateHalfOpen              // 2：探测，放行有限请求
)

func (s State) String() string {
    switch s {
    case StateClosed:   return "Closed"
    case StateOpen:     return "Open"
    case StateHalfOpen: return "HalfOpen"
    default:            return "Unknown"
    }
}
```

```
               失败率 ≥ FailureThreshold
               且 总请求数 ≥ MinRequests
    Closed ──────────────────────────────► Open
      ▲                                      │
      │                                      │ 超过 Timeout
      │                                      ▼
      │  连续成功 ≥ SuccessThreshold     HalfOpen
      └──────────────────────────────────────┤
                                             │ 失败
                                             ▼
                                           Open（重置计时器）
```

## 配置与初始化

**源码**：`pkg/circuitbreaker/breaker.go`

```go
type Config struct {
    MaxRequests      uint32        // Half-Open 状态最多放行的探测请求数，默认 1
    MinRequests      uint32        // 触发熔断所需最少请求样本，默认 10
    Interval         time.Duration // Closed 状态的统计周期，默认 60s
    Timeout          time.Duration // Open 状态持续时间（到期后进入 Half-Open），默认 60s
    FailureThreshold float64       // 失败率阈值（0.0–1.0），默认 0.5（50%）
    SuccessThreshold uint32        // Half-Open 状态：连续成功次数达到此值后恢复 Closed，默认 1
}

func NewCircuitBreaker(config Config) *CircuitBreaker {
    return &CircuitBreaker{
        config: config,
        state:  StateClosed,
        window: newSlidingWindow(config.Interval),
    }
}
```

## CircuitBreaker 结构

```go
type CircuitBreaker struct {
    mu      sync.Mutex
    state   State
    config  Config
    window  *SlidingWindow // 滑动时间窗口，统计成功/失败/超时

    // Half-Open 状态跟踪
    halfOpenRequests uint32  // 已放行的探测请求数
    halfOpenSuccess  uint32  // 连续成功次数

    expiry time.Time // Open 状态的到期时间
}
```

## 核心方法

### Allow()

```go
func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    now := time.Now()

    switch cb.state {
    case StateClosed:
        return true

    case StateOpen:
        if now.After(cb.expiry) {
            // Timeout 到期，转入 Half-Open
            cb.toHalfOpen()
            return true // 放行第一个探测请求
        }
        return false

    case StateHalfOpen:
        if cb.halfOpenRequests < cb.config.MaxRequests {
            cb.halfOpenRequests++
            return true
        }
        return false // 超过 MaxRequests，拒绝
    }
    return false
}
```

### RecordSuccess() / RecordFailure()

```go
func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.window.RecordSuccess()

    if cb.state == StateHalfOpen {
        cb.halfOpenSuccess++
        if cb.halfOpenSuccess >= cb.config.SuccessThreshold {
            cb.toClosed() // 达到成功阈值，恢复 Closed
        }
    }
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.window.RecordFailure()

    switch cb.state {
    case StateClosed:
        // 检查是否需要熔断
        if cb.shouldTrip() {
            cb.toOpen()
        }
    case StateHalfOpen:
        // 探测失败，回到 Open
        cb.toOpen()
    }
}

func (cb *CircuitBreaker) shouldTrip() bool {
    total, failures := cb.window.Stats()
    if total < int64(cb.config.MinRequests) {
        return false // 样本不足，不触发
    }
    rate := float64(failures) / float64(total)
    return rate >= cb.config.FailureThreshold
}
```

### 状态转换

```go
func (cb *CircuitBreaker) toOpen() {
    cb.state = StateOpen
    cb.expiry = time.Now().Add(cb.config.Timeout)
    cb.window.Reset()
}

func (cb *CircuitBreaker) toHalfOpen() {
    cb.state = StateHalfOpen
    cb.halfOpenRequests = 0
    cb.halfOpenSuccess = 0
}

func (cb *CircuitBreaker) toClosed() {
    cb.state = StateClosed
    cb.window.Reset()
    cb.halfOpenRequests = 0
    cb.halfOpenSuccess = 0
}
```

## SlidingWindow（滑动时间窗口）

**源码**：`pkg/circuitbreaker/window.go`（125 行）

滑动窗口将时间划分为 **10 个桶**，每个桶记录该时间段内的请求统计，过期桶自动清零：

```go
type SlidingWindow struct {
    buckets  [10]*Bucket    // 固定 10 个桶
    size     int            // 桶数量（= 10）
    interval time.Duration  // 整个窗口大小（如 60s）
    // 每个桶覆盖 interval/size 的时间段（如 6s）
    mu       sync.Mutex
}

type Bucket struct {
    Success int64
    Failure int64
    Timeout int64
    expiry  time.Time
}

func (w *SlidingWindow) RecordSuccess() {
    w.mu.Lock()
    defer w.mu.Unlock()
    bucket := w.currentBucket()
    atomic.AddInt64(&bucket.Success, 1)
}

func (w *SlidingWindow) RecordFailure() {
    w.mu.Lock()
    defer w.mu.Unlock()
    bucket := w.currentBucket()
    atomic.AddInt64(&bucket.Failure, 1)
}

func (w *SlidingWindow) RecordTimeout() {
    w.mu.Lock()
    defer w.mu.Unlock()
    bucket := w.currentBucket()
    atomic.AddInt64(&bucket.Timeout, 1)
}

// Stats 汇总所有有效桶的统计（过期桶已清零）
func (w *SlidingWindow) Stats() (total, failures int64) {
    w.mu.Lock()
    defer w.mu.Unlock()
    for _, b := range w.buckets {
        if b != nil && time.Now().Before(b.expiry) {
            total += b.Success + b.Failure + b.Timeout
            failures += b.Failure + b.Timeout // 超时也算失败
        }
    }
    return
}

// FailureRate 计算当前失败率
func (w *SlidingWindow) FailureRate() float64 {
    total, failures := w.Stats()
    if total == 0 {
        return 0
    }
    return float64(failures) / float64(total)
}
```

**10 桶设计**：60 秒窗口，每桶 6 秒，提供足够精度的同时内存占用极小。

## 服务端熔断拦截器

框架还提供了服务端使用的 `CircuitBreakerInterceptor`，防止服务端被异常客户端打垮：

```go
// pkg/circuitbreaker/breaker.go
func CircuitBreakerInterceptor(cb *CircuitBreaker) interceptor.Interceptor {
    return func(ctx context.Context, req *protocol.Request,
        next interceptor.Invoker) (interface{}, error) {

        if !cb.Allow() {
            return nil, &protocol.Error{
                Code:    protocol.Unavailable,
                Message: "circuit breaker is open",
            }
        }

        result, err := next(ctx, req)

        if err != nil {
            cb.RecordFailure()
        } else {
            cb.RecordSuccess()
        }
        return result, err
    }
}
```

## 在客户端使用（Discovery 模式）

客户端为每个服务实例地址维护独立的熔断器（见 [服务发现模式](../client/discovery-mode.md)）：

```go
cli, _ := client.NewDiscoveryClient(
    client.WithCircuitBreaker(true),
    // 自定义熔断器配置
    client.WithCircuitBreakerConfig(circuitbreaker.Config{
        MaxRequests:      3,
        MinRequests:      20,
        Interval:         30 * time.Second,
        Timeout:          10 * time.Second,
        FailureThreshold: 0.5,
        SuccessThreshold: 2,
    }),
)
```

## 获取熔断器状态

```go
cb := circuitbreaker.NewCircuitBreaker(config)

// 查看当前状态
fmt.Println(cb.State())         // "Closed" / "Open" / "HalfOpen"
fmt.Println(cb.FailureRate())   // 当前失败率，如 0.35
total, failures := cb.window.Stats()
```

## 配置建议

| 场景 | MinRequests | FailureThreshold | Timeout |
|------|-------------|-----------------|---------|
| 内部服务（宽松）| 20 | 60% | 5s |
| 对外 API（标准）| 10 | 50% | 10s |
| 关键路径（严格）| 5 | 30% | 30s |

## 相关文档

- [限流器](rate-limiter.md)
- [错误码](../protocol/error-codes.md) — Unavailable 错误
- [服务发现模式](../client/discovery-mode.md) — 每实例熔断器
- [拦截器链](../server/interceptors.md) — 服务端熔断拦截器
