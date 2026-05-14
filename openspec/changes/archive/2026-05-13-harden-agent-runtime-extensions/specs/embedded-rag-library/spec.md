## ADDED Requirements

### Requirement: Agent 支持显式 runtime extension 配置
系统 SHALL 允许调用方在 Agent 配置中显式启用 runtime extension 能力，包括 tool-calling runner 和 memory provider；未启用时 MUST 保持现有 RAG-first 稳定核心行为。

#### Scenario: 未启用扩展时兼容旧行为
- **WHEN** 调用方使用旧配置构造 Agent 并执行同步或流式问答
- **THEN** 系统 MUST 保持现有知识导入、检索、引用、流式和 fallback 工具行为不变

#### Scenario: 启用 tool-calling 后仍遵守检索边界
- **WHEN** 调用方启用 tool-calling runner 并为单次查询设置 source path、prefix 或 metadata filter
- **THEN** 系统 MUST 在检索和工具回灌中继续遵守该查询边界，不能因为 tool-calling 绕过已有 access boundary

#### Scenario: 启用 memory provider 后仍遵守 prompt 预算
- **WHEN** memory provider 返回历史消息或记忆文本
- **THEN** 系统 MUST 将其纳入现有 prompt token 预算裁剪，而不能无限扩展 prompt
