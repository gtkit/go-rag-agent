## Purpose

定义运行时观测能力，包括结构化执行 trace、稳定摘要视图、trace recorder adapter，以及 tool-calling 和 memory provider 的安全生命周期摘要。

## Requirements

### Requirement: ExecutionTrace 提供稳定摘要视图
系统 SHALL 提供一个稳定的 `ExecutionTrace` 摘要视图，便于日志、回归和外部系统消费。

#### Scenario: 生成同步问答 trace 摘要
- **WHEN** 调用方对一次同步问答的 `ExecutionTrace` 生成摘要
- **THEN** 系统 MUST 返回包含 success、duration、tool/provider 调用计数、citation 数量等关键聚合字段的摘要结构

#### Scenario: 生成流式问答 trace 摘要
- **WHEN** 调用方对一次流式问答的 `ExecutionTrace` 生成摘要
- **THEN** 系统 MUST 保留对 stream 语义的标识，而不要求调用方解析完整 trace

### Requirement: Trace recorder adapters 提供标准消费方式
系统 SHALL 提供标准 trace recorder adapters，使调用方可以把 `ExecutionTrace` 安全输出到多个目的地、JSONL 流或已有 logger，而不需要重写 recorder glue code。

#### Scenario: Multi recorder fan-out trace
- **WHEN** 调用方使用 multi trace recorder 包装多个 recorder
- **THEN** 系统 MUST 把每次 execution trace 发送给所有非 nil recorder，并 MUST 隔离单个 recorder 的 panic 或失败影响

#### Scenario: JSONL recorder 输出安全摘要
- **WHEN** 调用方使用 JSONL trace recorder 记录一次 execution trace
- **THEN** 系统 MUST 输出一行可解析 JSON，包含 session、成功状态、耗时、工具/provider 计数、fallback、citation 数量和错误摘要，但 MUST NOT 输出完整 prompt、API key 或完整 evidence 文本

#### Scenario: Logger recorder 复用现有 Logger 接口
- **WHEN** 调用方使用 logger trace recorder
- **THEN** 系统 MUST 通过现有 `Logger` 接口输出成功、失败和 fallback 摘要，不引入新的日志依赖

### Requirement: Tool-calling 和 memory provider 参与执行 trace
系统 SHALL 在结构化 execution trace 中记录 tool-calling runner 和 memory provider 的关键生命周期摘要。

#### Scenario: Tool-calling trace 包含工具摘要
- **WHEN** 一次问答通过 tool-calling runner 执行一个或多个工具
- **THEN** execution trace MUST 包含每个工具调用的名称、开始时间、结束时间、耗时和错误摘要

#### Scenario: Memory provider trace 包含检索和写入摘要
- **WHEN** memory provider 参与一次问答
- **THEN** execution trace MUST 包含 memory retrieve 和 memorize 的成功状态、耗时和错误摘要
