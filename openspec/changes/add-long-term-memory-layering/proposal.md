## Why

当前库已经有短期会话历史和本地摘要压缩，但一旦历史超出窗口预算，旧轮次只能被压缩进 prompt，无法作为可检索的长期记忆继续参与后续问答。要把这个包继续推进成更完整的 RAG Agent，需要在现有短期历史之上增加一个可插拔的长期记忆层。

## What Changes

- 在根包公开 `LongTermMemoryStore` 边界和相关数据类型
- 增加同 Session 的长期语义记忆检索，并在问答时把命中的长期记忆回灌到 prompt
- 在成功问答后把用户与助手对话写入长期记忆层
- 提供一个默认的 `in-memory` 长期记忆实现
- 更新 README 与测试，说明短期/长期记忆分层语义和注入方式

## Capabilities

### New Capabilities
- `long-term-memory-layering`: 定义短期历史与同 Session 语义长期记忆的分层行为、注入边界和默认 in-memory 实现

### Modified Capabilities
- `embedded-rag-library`: 问答流程在保持现有 API 不变的前提下增加长期记忆检索与写入

## Impact

- 受影响代码：`config.go`、`agent.go`、`session.go`、新增长期记忆存储与测试模块、`internal/graph/*`
- 受影响 API：新增 `LongTermMemoryStore`、长期记忆配置与默认构造器
- 受影响文档：`README.md`
