## ADDED Requirements

### Requirement: LangChainGo 执行后端保持核心搜索能力
系统 SHALL 使用 LangChainGo 执行后端完成问答编排，同时保持本地知识检索与联网搜索两条核心能力可用，且不改变现有公开 `Agent` / `Session` API。

#### Scenario: 本地知识检索优先参与问答
- **WHEN** 会话查询能够从本地知识库检索到足够证据
- **THEN** 系统 MUST 使用本地检索证据完成回答，而不是先走联网搜索

#### Scenario: 本地证据不足时允许联网搜索
- **WHEN** 会话查询无法从本地知识库检索到足够证据，且已启用联网搜索配置
- **THEN** 系统 MUST 进入联网搜索路径，并把搜索结果作为后续回答上下文的一部分

#### Scenario: 流式问答保留搜索能力
- **WHEN** 调用方使用流式问答接口发起查询
- **THEN** 系统 MUST 在流式路径中维持与同步问答一致的本地检索和联网搜索能力
