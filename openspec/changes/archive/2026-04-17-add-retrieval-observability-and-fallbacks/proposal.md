## Why

当前库已经具备企业级检索能力的雏形，但还缺两类线上运行必备基础：一是可观测性，二是降级保护。没有可观测性，线上无法判断 hybrid/rerank 的收益与成本；没有降级保护，一旦增强检索链路内部出错，就会把本来还能回答的问题直接打成失败。

## What Changes

- 为检索和模型执行增加可选的详细 metrics 回调。
- 为 hybrid / rerank 路径增加明确的 fallback 机制与通知事件。
- 保持现有 `Callback` 接口兼容，不强制现有调用方实现新方法。
- README 更新中文说明，补充观测点、降级语义和企业级使用建议。

## Capabilities

### New Capabilities

- `runtime-observability`: 暴露检索、模型和降级事件的详细运行时观测数据

### Modified Capabilities

- `embedded-rag-library`: 修改检索行为要求，使 hybrid / rerank 在内部失败时可以按阶段降级而不是直接中断

## Impact

- 受影响代码：`types.go`、`agent.go`、`internal/retrieval/*`、相关测试与 `README.md`
- 受影响行为：hybrid 失败时退回 vector-only，rerank 失败时退回 hybrid
- 不新增外部依赖
