## ADDED Requirements

### Requirement: 根包公开稳定的 Tool 与 ToolRegistry 边界
系统 SHALL 在根包公开稳定的 `Tool` 和 `ToolRegistry`，使调用方可以在不依赖 `internal/tools` 的前提下注册、覆写和枚举工具。

#### Scenario: 调用方可注册自定义工具
- **WHEN** 调用方实现根包公开的 `Tool` 接口并将其注册到 `ToolRegistry`
- **THEN** 该工具 MUST 可以被 `Agent` 配置接收，而不需要调用方 import `internal/tools`

#### Scenario: 注册表拒绝重复名称
- **WHEN** 调用方尝试向注册表中注册两个同名工具
- **THEN** 注册表 MUST 返回明确错误，而不是静默覆盖

### Requirement: 默认工具通过统一注册表接线
系统 SHALL 将默认 `retrieve_context` 与可选 `search_web` 工具纳入统一注册表构建，而不是保留分散的硬编码接线。

#### Scenario: 默认配置继续拥有本地检索工具
- **WHEN** 调用方使用默认配置创建 Agent
- **THEN** 库 MUST 继续注册并使用 `retrieve_context` 工具

#### Scenario: 启用联网搜索时继续拥有默认 web 工具
- **WHEN** 调用方启用 `EnableWebSearch`
- **THEN** 库 MUST 继续注册并使用默认 `search_web` 工具

#### Scenario: 调用方可以覆写默认 web 工具
- **WHEN** 调用方显式注册同名的 `search_web` 工具作为配置输入
- **THEN** 库 MUST 使用调用方提供的同名工具，而不是继续使用默认实现

### Requirement: `MaxToolCalls` 必须真实约束执行
系统 SHALL 让 `MaxToolCalls` 对当前工具执行链真实生效，而不是仅作为保留字段存在。

#### Scenario: 本地检索计入工具调用次数
- **WHEN** 一次问答执行本地检索
- **THEN** 该次 `retrieve_context` 调用 MUST 计入 `MaxToolCalls`

#### Scenario: fallback 工具尝试计入工具调用次数
- **WHEN** 本地证据为空，执行进入 fallback 工具链
- **THEN** 每次工具尝试 MUST 各自计入 `MaxToolCalls`

#### Scenario: 超出上限时停止继续调用工具
- **WHEN** 执行中的工具调用次数达到 `MaxToolCalls`
- **THEN** 库 MUST 停止继续调用后续工具，并返回受控错误，而不是继续执行

### Requirement: 工具注册与调用上限不破坏现有可观测性
系统 SHALL 让注册工具与默认工具继续走现有 tool lifecycle telemetry 和 execution trace。

#### Scenario: 注册工具继续发出 tool lifecycle telemetry
- **WHEN** 已注册工具在问答过程中被执行
- **THEN** 库 MUST 继续发出 `OnToolStart` / `OnToolEnd` 和对应 stream 事件

#### Scenario: 注册工具继续进入 execution trace
- **WHEN** 已注册工具在问答过程中被执行
- **THEN** 该工具调用 MUST 继续记录到 `ExecutionTrace.ToolCalls`
