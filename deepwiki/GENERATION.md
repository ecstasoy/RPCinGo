# Generation Metadata

| 字段 | 值 |
|------|-----|
| Commit Hash | `f99e580` |
| Branch | `main` |
| 生成时间 | 2026-04-04 |
| 生成工具 | deepwiki 0.1.0 |
| 语言 | 中文 |

## 增量更新（2026-04-04，commit f99e580）

- 新增 `modules/logger.md`：pkg/logger 完整模块页（接口、构造函数、集成点、自定义实现）
- 更新 `modules/interceptor.md`：Logging 拦截器迁移至 `logger.Logger`，日志格式改为 slog key-value
- 更新 `modules/server.md`：Options 表新增 `WithLogger(l)` 条目
- 更新 `modules/pool.md`：PoolOptions 新增 `Logger` 字段说明和 `WithPoolLogger` 示例
- 更新 `INDEX.md`：配置与学习分类新增 Logger 模块链接

## 增量更新（2026-04-04，commits 4029347–f5d51b7）

- 更新 `modules/server.md`：`Stop()` 生命周期图补充 `sync.Once` 幂等关闭说明（Bug 10）
- 更新 `modules/transport.md`：codec 方法签名注释说明使用 `writeFull` 保证完整写入；边界情况新增"写入不完整"条目（Bug 11）
- 更新 `modules/interceptor.md`：`buildChain` 闭包代码注释说明 `:=` 创建每次迭代独立变量，在所有 Go 版本中均正确（Bug 12 分析）

## 增量更新（2026-04-04）

- 新增 `modules/tracing.md`：覆盖 `pkg/tracing/`（新包）+ `pkg/interceptor/tracing.go`（TracingClient/TracingServer/metadataCarrier）
- 更新 `modules/client.md`：`CallTyped` 返回值修正为 `(*protocol.Response, error)`，补充 SpanID 读取示例
- 更新 `modules/server.md`：补充 `WithRateLimit` option、HandleRequest 中 SpanID 回写逻辑
- 更新 `INDEX.md`：新增 Tracing 模块链接，分类重命名为"可靠性与可观测性"

## 初次生成（2026-03-31）

- 从 `wiki/` 迁移全部内容并按 deepwiki 模板重构
- 新增 mermaid 流程图、参数表、示例代码片段
- 新增 guides/ 目录：配置、测试、可观测性
- 生成统一 INDEX.md 索引

## 增量更新说明

下次运行 `/deepwiki` 时，工具将对比此 commit hash 与最新 commit，仅更新有变动的模块。
