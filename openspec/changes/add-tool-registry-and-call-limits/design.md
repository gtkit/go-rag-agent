## Context

当前库的工具能力仍然是半封闭的：

- 默认工具只有 `retrieve_context` 和 `search_web`
- 工具接口定义在 `internal/tools`
- `ChatRunner` 只会特殊识别 `search_web`
- `MaxToolCalls` 虽然在配置中存在，但没有真正控制执行

如果要把这个库继续向“可扩展的 agent/RAG runtime”推进，至少需要先把工具边界公开，并把调用上限从配置占位变成真实执行约束。

## Goals / Non-Goals

**Goals:**
- 在根包公开稳定的 `Tool` 与 `ToolRegistry`
- 把默认工具集合接入统一注册表
- 让 `MaxToolCalls` 真实约束检索与 evidence-empty fallback 工具链
- 保持现有默认行为、trace 和 telemetry 语义不回归

**Non-Goals:**
- 不在这轮实现完整 ReAct/tool-calling agent loop
- 不引入模型驱动的任意工具选择协议
- 不改变现有 `Ask` / `AskStream` 的公开调用方式

## Decisions

### 1. 公开根包 Tool 接口，但保持当前最小契约

根包公开的 `Tool` 继续采用当前最小模型：
- `Name() string`
- `Description() string`
- `Run(ctx, input) (string, error)`

这样调用方可以注册工具，同时不会引入复杂 schema 或 structured invocation 协议。

### 2. ToolRegistry 以“按名称注册 + 有序返回”工作

注册表负责：
- 保存工具
- 检查重名
- 暴露有序工具集合

默认工具也通过注册表构建，这样默认工具和自定义工具走同一条接线逻辑。

### 3. 当前执行模型只支持“fallback 工具链”，不做任意工具循环

这轮不实现完整 generic tool loop。可执行范围限定为：
- 本地检索仍然是第一步
- 当 evidence 为空时，通过注册表里的 fallback 工具链按顺序尝试工具
- 默认 `search_web` 是其中一个 fallback 工具

这样既让注册表有实际用途，也不把范围扩大到完整 agent planner。

### 4. `MaxToolCalls` 计入检索与 fallback 工具调用

工具调用次数计数规则：
- `retrieve_context` 算一次
- 每次 fallback 工具尝试各算一次
- 超过上限时停止继续调用工具并返回受控错误

默认 `MaxToolCalls=4` 维持现状；但当调用方显式设置更小值时，执行现在必须真的受限。

### 5. Trace / telemetry 继续沿用现有协议

自定义工具和默认工具都通过现有：
- `OnToolStart` / `OnToolEnd`
- `EventToolStart` / `EventToolEnd`
- `ExecutionTrace.ToolCalls`

这样上层可观测性不需要另开协议。

## Risks / Trade-offs

- [工具注册表存在，但没有完整模型驱动 tool loop] → 在 README 和 spec 中明确这轮只支持 fallback 工具链
- [自定义工具顺序影响行为] → 注册表保持有序，文档说明顺序语义
- [`MaxToolCalls` 从 no-op 变成真实生效，可能影响旧调用方] → 默认值不变，并通过兼容测试锁住默认路径

## Migration Plan

1. 先补 proposal/design/spec/tasks
2. 先写失败测试，锁定工具注册与调用上限语义
3. 公开根包 `Tool` / `ToolRegistry` 和配置注入
4. 调整默认工具接线与 `ChatRunner` fallback 逻辑
5. 让 `MaxToolCalls` 生效
6. 更新 README、OpenSpec 任务状态并跑完整验证

## Open Questions

- 无。完整 generic tool loop 已明确排除在本 change 外。
