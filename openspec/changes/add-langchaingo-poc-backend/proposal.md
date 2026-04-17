## Why

当前运行时把模型适配、工具调用协议和 ReAct 编排都绑定在 Eino 上，依赖面偏大，替换或裁剪执行后端的成本偏高。需要在不改变现有公开 API 的前提下，验证一个更轻的 LangChainGo 执行后端是否能稳定覆盖本地知识检索和联网搜索两条核心路径。

## What Changes

- 将内部问答执行后端从 Eino ReAct PoC 切换为 LangChainGo 实现，保留现有 `Agent`、`Session`、知识导入和查询接口不变。
- 保持本地知识检索工具和 Tavily 联网搜索工具都可参与问答流程。
- 为新的执行后端补充回归测试，覆盖同步与流式路径中的本地检索和联网搜索能力。
- 更新 README，说明当前实验分支的后端实现与约束。

## Capabilities

### New Capabilities
- `langchaingo-execution-backend`: 定义使用 LangChainGo 执行后端时，本地检索与联网搜索路径必须保持的运行时行为

### Modified Capabilities
- `embedded-rag-library`: 问答执行后端替换后仍需保持现有库优先 API、同步问答和流式问答语义
- `web-search-tool`: 联网搜索工具在新执行后端下仍需按现有条件启用并返回可供模型消费的文本

## Impact

- 受影响代码：`agent.go`、`internal/graph/*`、`internal/tools/*`、`internal/llm/*`、相关测试
- 受影响依赖：新增 `github.com/tmc/langchaingo`，移除或缩减 `github.com/cloudwego/eino*` 依赖
- 受影响文档：`README.md`
