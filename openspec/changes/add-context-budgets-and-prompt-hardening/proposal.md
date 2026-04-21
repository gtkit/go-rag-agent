## Why

当前库已经有会话历史、工具调用上限和请求超时，但还缺少真正的上下文窗口治理与 prompt 安全层：历史只是按轮次裁剪，证据没有 token 预算，工具回灌和检索文本也没有专门的 prompt injection 硬化。要把这个包继续向生产级 RAG Agent 推进，需要先把这些执行约束和安全边界补上。

## What Changes

- 增加 prompt token 预算能力，覆盖总 prompt、历史、证据和历史摘要预算
- 为被裁掉的旧历史提供本地摘要压缩，而不是只按轮次丢弃
- 增加总执行时长预算，使一次问答可以整体受控而不只是依赖单次 HTTP timeout
- 为检索证据和工具回灌文本增加基础 prompt hardening，明确把它们视为不可信输入
- 保持现有 `Ask` / `AskStream` API 不变，在默认路径中自动应用治理能力

## Capabilities

### New Capabilities
- `context-budgets-and-prompt-hardening`: 定义 prompt token 预算、历史压缩、总执行时长预算和 prompt hardening 的运行时语义

### Modified Capabilities
- `embedded-rag-library`: 同步与流式问答在现有 RAG 链上增加上下文治理和安全硬化，但不改变对外 API

## Impact

- 受影响代码：`config.go`、`agent.go`、`session.go`、`internal/graph/*`、相关测试
- 受影响依赖：新增 `tiktoken-go` 的直接使用
- 受影响文档：`README.md`
