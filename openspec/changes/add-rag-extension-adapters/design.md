## Context

本项目当前定位是 Go SDK / embeddable runtime，不是独立控制台。`minrag` 提供了许多应用级能力：组件流水线、后台管理、文档转换、FTS、MCP、OpenAI-compatible API。适合借鉴的是 SDK 级扩展边界，不适合直接搬后台和数据库管理面。

现有代码已经具备：
- `DocumentLoader` 默认导入 `.txt` / `.md` / `.html` / `.pdf` / 图片桥接
- `Reranker` 接口和 hybrid/rerank 降级路径
- `Tool` / `ToolRegistry` fallback 工具链
- `examples/service` 嵌入式服务接线

## Goals / Non-Goals

**Goals:**
- 提供文档转换接口与命令型转换器，支持默认 loader 外的格式预处理
- 提供 OpenAI-compatible reranker 适配器，保留现有 `Reranker` 契约
- 提供 MCP 工具适配器，保留现有 `Tool` 契约
- 提供轻量 Gateway 示例，展示 Chat Completions 嵌入方式
- 更新 README / Example tests，明确能力边界

**Non-Goals:**
- 不引入后台 UI、安装页、用户表、主题模板或通用 CRUD
- 不引入完整 workflow/pipeline 编排框架
- 不修改现有 `Tool`、`Reranker`、`Agent` 方法签名
- 不绑定具体商业 provider 的专有 SDK
- 不引入新的硬依赖数据库或 CGO FTS5

## Decisions

### 1. 文档转换走可选接口链

新增根包接口：
- `DocumentConverter`：声明 `Name()`、`Supports(ctx, path)`、`Convert(ctx, path)`
- `DocumentConverterFunc` 或命令型 `NewCommandDocumentConverter(...)`

`Config.DocumentConverters []DocumentConverter` 注入到 `Agent`。导入文件时先按顺序尝试 converter；只有 converter 明确支持该文件才执行转换。若全部不支持，再走默认 `DocumentLoader`。

转换结果复用现有 `Document`，metadata 继续进入现有 sidecar/front matter 合并流程。

### 2. 命令型转换器作为最小默认实现

命令型转换器只依赖标准库：
- 支持扩展名白名单
- 支持 `{input}` / `{output}` 占位符
- 执行命令时透传调用方 `context.Context`
- 读取输出 markdown/text 文件生成 `Document`

这样可以覆盖 markitdown / tika-wrapper / 自研 HTML cleaner 等本地工具，又避免绑定外部工具依赖。

### 3. OpenAI-compatible reranker 适配器独立实现

新增 `OpenAIRerankerConfig` 与 `NewOpenAIReranker(httpClient, cfg)`。它实现现有 `Reranker`：
- 发送 query + candidates 到配置 endpoint
- 只发送 shortlist
- 按返回 index/score 重排
- HTTP/JSON/索引错误均返回错误，交由既有 rerank fallback 处理

不在本次添加具体平台默认模型常量，避免编造 provider 细节。

### 4. MCP 适配器实现现有 Tool

新增 `MCPToolConfig` 与 `NewMCPTool(httpClient, cfg)`。适配器实现 `Tool`：
- `Name` / `Description` 来自配置
- `Run(ctx, input)` 发送 JSON-RPC 请求
- 返回文本结果或包装错误

本次只覆盖 HTTP JSON-RPC 形态，不做 stdio MCP session 管理和工具自动发现，避免把工具系统扩张成完整 agent framework。

### 5. Gateway 放在 examples，不进入核心 server

新增 `examples/gateway` 展示标准库 `net/http` handler：
- 请求结构只覆盖非流式 chat completions 的最小字段
- 从 `user` 或 fallback session ID 获取 Session
- 取最后一条 user 消息调用 `Ask`
- 返回 OpenAI-compatible 响应外形

核心包不新增 Gin/Hertz handler，保持 SDK 无 web framework 依赖。

## Risks / Trade-offs

- [转换器 API 面积增加] → 只添加可选接口，不改变默认 loader 行为
- [命令型转换器资源泄漏] → 使用 `exec.CommandContext`、临时目录、defer 清理
- [外部 reranker schema 差异] → 提供 OpenAI-compatible 通用实现，其他 provider 通过实现 `Reranker` 自行适配
- [MCP 协议覆盖不完整] → 文档明确首版只支持 HTTP JSON-RPC 工具调用
- [Gateway 被误解为内置平台] → README 明确示例边界

## Migration Plan

1. 先补失败测试：converter 链、命令转换器、OpenAI reranker、MCP tool、Gateway 示例请求
2. 实现最小接口与适配器
3. 补 README、Example tests 和 Gateway 示例
4. 运行 OpenSpec validate、lint、vet、race tests

## Open Questions

- 无
