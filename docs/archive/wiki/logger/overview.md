# 日志模块（Logger）

## 概述

`pkg/logger` 是 RPCinGo 的统一日志抽象层。框架内部所有日志调用均通过此接口，用户可替换为任意实现（zap、zerolog、logrus 等）。

默认实现基于 Go 标准库 `log/slog`（Go 1.21+），零外部依赖。

**源码位置**：`pkg/logger/logger.go`

## Logger 接口

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger  // 返回带固定字段的子 Logger
}
```

使用 slog 风格的 key-value 可变参数，例如：

```go
l.Info("rpc call ok", "service", "Calculator", "method", "Add", "duration", 1*time.Millisecond)
// 输出：time=... level=INFO msg="rpc call ok" service=Calculator method=Add duration=1ms
```

## 构造函数

```go
// 默认：slog 文本格式，输出到 stderr，INFO 及以上
l := logger.New()

// 自定义级别（开发时启用 DEBUG）
l := logger.NewWithLevel(slog.LevelDebug)

// 静默（用于测试或不需要日志的组件）
l := logger.Nop()
```

## 在框架中注入

### Server

```go
srv := server.NewServer(
    server.WithLogger(logger.New()), // 不传则默认 logger.New()
)
```

Server 的 `stopHeartbeat` 错误日志会输出到此 Logger：

```
time=... level=ERROR msg="heartbeat stopped" error="connection refused"
```

### Connection Pool

```go
pool, _ := pool.NewConnectionPool("127.0.0.1:8080",
    pool.WithPoolLogger(logger.New()), // 不传则默认 logger.Nop()（静默）
)
```

连接池默认静默，配置告警（如 MinSize 过大）只在显式传入 Logger 时输出：

```
time=... level=WARN msg="pool config: MinSize > MaxSize/2, may waste resources" minSize=60 maxSize=100
time=... level=WARN msg="pool: pre-creation of connection failed" error="dial tcp: connection refused"
```

### Logging 拦截器

```go
// 传 nil 自动使用 logger.New()
srv.Use(interceptor.Logging(nil))

// 传入自定义 Logger
l := logger.NewWithLevel(slog.LevelDebug)
srv.Use(interceptor.Logging(l))
```

## 自定义实现（适配 zap）

```go
import "go.uber.org/zap"

type ZapLogger struct{ s *zap.SugaredLogger }

func (z *ZapLogger) Debug(msg string, args ...any) { z.s.Debugw(msg, args...) }
func (z *ZapLogger) Info(msg string, args ...any)  { z.s.Infow(msg, args...) }
func (z *ZapLogger) Warn(msg string, args ...any)  { z.s.Warnw(msg, args...) }
func (z *ZapLogger) Error(msg string, args ...any) { z.s.Errorw(msg, args...) }
func (z *ZapLogger) With(args ...any) logger.Logger {
    return &ZapLogger{z.s.With(args...)}
}

zapCore, _ := zap.NewProduction()
l := &ZapLogger{zapCore.Sugar()}

srv := server.NewServer(server.WithLogger(l))
```

## 相关文档

- [拦截器链](../server/interceptors.md) — Logging 拦截器详解
- [Server 概述](../server/overview.md) — WithLogger option
- [连接池](../transport/connection-pool.md) — WithPoolLogger option
