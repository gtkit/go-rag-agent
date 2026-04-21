## Why

当前库已经具备存储、工具、结构化输出、上下文治理等能力，但 provider 层仍然偏薄：聊天、embedding 和联网搜索基本只有 timeout，没有统一的错误分类、重试/退避，也没有真正聚合 usage/cost。对生产级 Agent 来说，这会直接影响线上稳定性和成本可观测性。

## What Changes

- 为聊天模型、embedding 和联网搜索增加统一的 provider 错误分类
- 为可安全重试的 provider 调用增加重试/退避能力
- 为模型与外部 provider 调用增加 usage/cost 聚合，并汇总到 execution trace
- 保持当前公开 `Ask` / `AskStream` API 不变
- 暂不引入 provider 限流和断路器

## Capabilities

### New Capabilities
- `provider-resilience-and-usage`: 定义 provider 错误分类、重试/退避与 usage/cost 聚合的运行时语义

### Modified Capabilities
- `runtime-observability`: execution trace 扩展到 provider usage / cost 聚合

## Impact

- 受影响代码：`config.go`、`trace.go`、`internal/llm/*`、`internal/websearch/*`、`agent.go`、相关测试
- 受影响文档：`README.md`
