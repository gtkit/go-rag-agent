## ADDED Requirements

### Requirement: 库支持可插拔的长期记忆层
系统 SHALL 在当前短期 Session 历史之外，提供一个可插拔的长期记忆层接口，使成功问答后的历史轮次可以被后续语义检索。

#### Scenario: 调用方可注入长期记忆实现
- **WHEN** 调用方通过配置提供 `LongTermMemoryStore`
- **THEN** 库 MUST 使用该实现进行长期记忆写入与检索，而不要求调用方改动现有问答 API

#### Scenario: 未注入时默认不启用长期记忆
- **WHEN** 调用方未配置长期记忆实现
- **THEN** 库 MUST 保持当前行为，不强制创建长期记忆依赖

### Requirement: 成功问答后写入长期记忆
系统 SHALL 在同步或流式问答成功完成后，将当前轮次写入长期记忆层。

#### Scenario: 同步问答成功后写入长期记忆
- **WHEN** 一次同步问答成功完成
- **THEN** 库 MUST 把该轮用户问题与助手回答写入长期记忆

#### Scenario: 流式问答成功后写入长期记忆
- **WHEN** 一次流式问答成功完成
- **THEN** 库 MUST 把该轮用户问题与最终拼接后的助手回答写入长期记忆

### Requirement: 后续问答可检索同 Session 的长期记忆
系统 SHALL 在后续问答时按当前 query 检索同 Session 的长期记忆，并将命中结果单独回灌到 prompt。

#### Scenario: 同 Session 旧轮次可被语义召回
- **WHEN** 当前 query 与该 Session 中较早的历史轮次存在语义相关性
- **THEN** 库 MUST 能召回这些长期记忆，即使它们已超出短期历史窗口

#### Scenario: 长期记忆与即时证据分开注入
- **WHEN** 问答同时命中当前知识证据与长期记忆
- **THEN** 库 MUST 把长期记忆作为独立上下文块注入，而不是直接混入当前知识证据字段

### Requirement: 默认 in-memory 长期记忆实现可用于单进程场景
系统 SHALL 提供默认 in-memory 长期记忆实现，用于单进程、同会话的长期语义记忆场景。

#### Scenario: in-memory 实现支持写入与检索
- **WHEN** 调用方使用默认 in-memory 长期记忆实现
- **THEN** 它 MUST 支持基本的写入、同 Session 检索、清理和关闭能力
