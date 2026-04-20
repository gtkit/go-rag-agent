## Why

当前库虽然已经切到 LangChainGo 执行后端，但默认 provider 构造、运行时观测和回归验证仍然偏“实现细节导向”：`Agent.New` 直接接线内部默认模型适配器，单次执行的信息分散在多个 callback 中，而且仓库缺少一组不依赖真实外部模型的确定性回归评测。继续迭代 provider、trace 或执行链路前，需要先把这些基础边界收紧。

## What Changes

- 在根包公开稳定的 runtime 抽象与默认 OpenAI-compatible 构造器，使调用方可以注入自定义 `ChatModel` / `Embedder`，同时保留当前默认 LangChainGo 适配路径。
- 为单次问答执行增加结构化 trace，覆盖 query rewrite、检索过滤条件、tool 调用、retrieval/model metrics、fallback、citation 与最终结果，并允许通过可选 recorder 和 logger 消费。
- 为仓库补充确定性的回归评测集，使用固定 stub 锁定 runtime 注入、同步 trace 和流式 trace 行为，不依赖真实外部 LLM/embedding 网络调用。
- 更新 README，说明新的 runtime 注入、trace 与回归测试使用方式。

## Capabilities

### New Capabilities
- `runtime-component-boundaries`: 定义根包公开的 runtime 抽象、默认适配器构造器与 Agent 运行时组件注入语义
- `evaluation-regression-suite`: 定义仓库内维护的确定性回归评测集以及它需要覆盖的核心场景

### Modified Capabilities
- `runtime-observability`: 运行时观测从分散的 lifecycle callback 扩展到单次执行 trace 与可选日志摘要

## Impact

- 受影响代码：`types.go`、`config.go`、`agent.go`、`session.go`、`internal/graph/*`、`internal/llm/*`、相关测试
- 受影响 API：`Config`、`Answer`、`StreamEvent` 新增 runtime 注入 / trace 相关字段与类型
- 受影响文档：`README.md`
