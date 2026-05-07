## Why

与 `minrag` 对比后，当前 SDK 已经具备核心 RAG runtime 能力，但在“把外部解析器、外部 reranker、MCP 工具和轻量 Gateway 嵌入现有 Go 服务”这几个扩展点上还缺少稳定收口。现在补齐这些 SDK 级适配能力，可以借鉴 `minrag` 的实用组件生态，同时避免把项目扩张成后台控制台或独立平台。

## What Changes

- 新增文档转换扩展点，让 Office / 复杂 PDF / 网页正文清洗等外部转换器可以在知识导入前接入。
- 新增 OpenAI-compatible reranker 适配器，复用现有 `Reranker` 接口，方便接入百炼、千帆、LKE 等兼容 HTTP rerank 服务。
- 新增 MCP-to-Tool 适配器，把远端 MCP 工具以现有 `ToolRegistry` 契约注册到 fallback 工具链。
- 新增轻量 OpenAI-compatible Chat Completions handler 示例，展示如何把 SDK 嵌入已有 HTTP 服务。
- 更新 README / Example tests，明确这些能力属于 SDK 扩展适配，不引入后台管理 UI、用户系统或控制台。

## Capabilities

### New Capabilities
- `rag-extension-adapters`: 定义文档转换、外部 reranker、MCP 工具适配和轻量 Gateway 示例的 SDK 扩展语义。

### Modified Capabilities
- `embedded-rag-library`: 知识导入流程允许调用方在内置 loader 前注入文档转换器，检索流程允许通过外部 reranker 适配器参与 shortlist 重排。
- `web-search-tool`: fallback 工具链可接入 MCP 工具适配器，但仍保持现有 `Tool` / `ToolRegistry` 契约。
- `example-tests`: Example tests 增加扩展适配与 Gateway 嵌入示例。

## Impact

- 受影响代码：`config.go`、`agent.go`、`source.go`、`document_loader.go`、新增 converter / reranker / mcp / gateway 示例文件、`README.md`、`example_test.go`。
- 受影响 API：新增可选接口与构造函数；不移除现有公开 API，不改变 `Tool`、`Reranker`、`Agent` 的已有方法签名。
- 受影响系统：知识导入、hybrid/rerank 检索、fallback 工具链、示例 HTTP 嵌入路径。
