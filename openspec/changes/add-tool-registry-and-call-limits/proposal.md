## Why

当前库虽然已经有工具生命周期事件和默认的 `retrieve_context` / `search_web` 两个工具，但工具集合仍然是硬编码的，调用方无法通过根包稳定注入或覆写工具。同时 `MaxToolCalls` 已经是公开配置，却还没有真正参与执行控制，需要把这两个缺口补齐。

## What Changes

- 在根包公开稳定的 `Tool` 与 `ToolRegistry` 边界，使调用方可以注册或覆写工具，而不依赖 `internal/tools`
- 将默认工具集合纳入统一注册表，而不是散落在 `Agent.New` 的接线逻辑中
- 让 `MaxToolCalls` 对实际执行生效，至少约束本地检索与 evidence-empty fallback 工具链
- 保持现有默认行为不变：默认仍然提供 `retrieve_context` 与可选的 `search_web`
- 更新 README 与测试，说明工具注册表、默认工具和调用上限语义

## Capabilities

### New Capabilities
- `tool-registry-execution`: 定义根包公开的工具注册边界、默认工具接线和工具调用上限语义

### Modified Capabilities
- `embedded-rag-library`: 问答执行路径增加公开工具注册入口，并使 `MaxToolCalls` 对执行控制生效

## Impact

- 受影响代码：`config.go`、`agent.go`、根包类型定义、`internal/tools/*`、`internal/graph/*`、相关测试
- 受影响 API：新增根包 `Tool`、`ToolRegistry`、工具注入配置；`MaxToolCalls` 从保留字段变为生效字段
- 受影响文档：`README.md`
