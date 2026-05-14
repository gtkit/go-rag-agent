## Purpose

定义 Agent runtime extension 的结构化工具、显式 tool-calling、memory provider 和安全默认值。

## Requirements

### Requirement: 结构化工具保持向后兼容
系统 SHALL 提供结构化工具契约，使工具可以声明参数 schema、执行结构化参数并返回结构化结果，同时 MUST 保持现有 `Tool` 接口和 `ToolRegistry` 注册语义可用。

#### Scenario: 结构化工具可适配为现有 Tool
- **WHEN** 调用方把一个有效的结构化工具适配为现有 `Tool`
- **THEN** 系统 MUST 保留工具名称、描述和执行能力，并允许它注册到现有 `ToolRegistry`

#### Scenario: 参数缺失时拒绝执行
- **WHEN** 结构化工具收到缺少 required 字段的 JSON 参数
- **THEN** 系统 MUST 在调用工具业务逻辑前返回可识别的参数校验错误

#### Scenario: 现有 Tool 不需要实现结构化接口
- **WHEN** 调用方继续使用只实现 `Tool` 的旧工具
- **THEN** 系统 MUST 保持旧工具的注册和执行行为不变

### Requirement: Tool-calling runner 显式启用且受预算约束
系统 SHALL 提供可选 tool-calling runner，使模型可以在单次问答中请求有限工具调用；该 runner MUST 仅在调用方显式启用时参与执行，并 MUST 同时遵守 `MaxToolCalls`、`MaxIterations` 和 context 取消。

#### Scenario: 默认问答路径不启用 tool-calling
- **WHEN** 调用方没有显式启用 tool-calling runner
- **THEN** 系统 MUST 继续使用现有 RAG-first 问答路径，且 MUST NOT 改变旧工具 fallback 语义

#### Scenario: 模型请求已注册工具时执行工具并继续生成
- **WHEN** tool-calling runner 收到模型输出的有效工具调用请求，且工具已注册、参数有效、预算未耗尽
- **THEN** 系统 MUST 执行该工具，把工具结果回灌下一轮模型请求，并最终返回模型答案

#### Scenario: 未知工具被拒绝
- **WHEN** 模型请求调用未注册工具
- **THEN** 系统 MUST 返回明确错误，并在 execution trace 中记录该失败

#### Scenario: 工具调用超过预算时中断
- **WHEN** 单次问答中的工具调用次数超过 `MaxToolCalls` 或迭代次数超过 `MaxIterations`
- **THEN** 系统 MUST 中断本次问答并返回明确的预算错误，而不是继续执行工具

#### Scenario: context 取消时停止工具循环
- **WHEN** 调用方传入的 context 在工具循环中取消
- **THEN** 系统 MUST 停止后续模型和工具调用，并返回 context 错误

### Requirement: Memory provider 可注入问答生命周期
系统 SHALL 允许调用方注入 memory provider，在问答前检索可注入上下文，在问答成功后写入本轮记忆，并 MUST 保持现有长期记忆 store 路径兼容。

#### Scenario: 问答前注入 provider 返回的记忆上下文
- **WHEN** 调用方配置 memory provider 且 provider 检索成功
- **THEN** 系统 MUST 把 provider 返回的记忆上下文纳入本轮 prompt 预算内的上下文构造

#### Scenario: 问答成功后写入 provider
- **WHEN** 一次同步或流式问答成功生成最终答案
- **THEN** 系统 MUST 调用 memory provider 写入本轮 user query 与 assistant answer

#### Scenario: provider 检索失败时按配置处理
- **WHEN** memory provider 在检索阶段返回错误
- **THEN** 系统 MUST 根据配置选择返回错误或降级为空记忆，并 MUST 在 trace 中记录该事件

#### Scenario: 现有 LongTermMemoryStore 可适配为 provider
- **WHEN** 调用方使用现有 `LongTermMemoryStore` 构造 memory provider adapter
- **THEN** 系统 MUST 通过该 adapter 复用现有长期记忆的存储、检索、TTL 和去重语义

### Requirement: 新增能力具备生产安全默认值
系统 SHALL 为结构化工具、tool-calling 和 memory provider 提供安全默认行为，避免未显式配置时产生额外外部调用、无限循环或敏感信息泄漏。

#### Scenario: 未配置工具时启用 tool-calling 返回配置错误
- **WHEN** 调用方启用 tool-calling 但没有注册任何可调用工具
- **THEN** 系统 MUST 在构造或首次执行时返回清晰配置错误

#### Scenario: trace 不记录完整敏感上下文
- **WHEN** tool-calling 或 memory provider 参与一次问答
- **THEN** 系统 MUST 在默认 trace 摘要中记录名称、计数、耗时和错误类别，但 MUST NOT 默认记录完整 prompt、API key 或未脱敏工具参数
