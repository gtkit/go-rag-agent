## 目的

定义 RAG SDK 的扩展适配能力，使调用方可以在不引入完整控制台的前提下接入外部文档转换器、外部 reranker、MCP 工具和轻量 Gateway 示例。

## ADDED Requirements

### Requirement: 文档导入支持外部转换器
系统 SHALL 允许调用方在知识导入流程中注册外部文档转换器，将 SDK 默认 loader 不支持或需要预处理的文件转换成可导入文档。

#### Scenario: 外部转换器成功处理受支持文件
- **WHEN** 调用方导入一个 SDK 默认 loader 不支持但外部转换器支持的文件
- **THEN** 系统 MUST 调用该转换器，并把转换结果送入常规分块、metadata 和 embedding 流程

#### Scenario: 转换器声明不支持时继续后续策略
- **WHEN** 已注册的转换器对某文件返回不支持
- **THEN** 系统 MUST 尝试后续转换器或默认 loader，而不是把不支持误判为导入失败

#### Scenario: 转换器执行失败时返回清晰错误
- **WHEN** 外部转换器声明支持某文件但转换执行失败
- **THEN** 系统 MUST 返回带文件路径和转换器信息的错误，且 MUST NOT 静默导入空内容

### Requirement: 外部 reranker 适配器复用现有 Reranker 契约
系统 SHALL 提供外部 reranker 适配器，使兼容 HTTP rerank 服务可以参与当前 hybrid shortlist 重排。

#### Scenario: 外部 reranker 返回新排序
- **WHEN** 调用方启用 `EnableRerank` 并注入外部 reranker 适配器
- **THEN** 系统 MUST 只把 shortlist 候选发送给外部 reranker，并按服务返回顺序生成最终候选

#### Scenario: 外部 reranker 响应非法时触发既有降级
- **WHEN** 外部 reranker 返回无效候选索引或 HTTP 调用失败
- **THEN** 系统 MUST 把错误返回给现有 rerank 降级路径，由主流程退回未 rerank 的 hybrid 结果

### Requirement: MCP 工具可适配为 ToolRegistry 工具
系统 SHALL 提供 MCP-to-Tool 适配器，把远端 MCP 工具注册为现有 `Tool`。

#### Scenario: MCP 工具执行成功
- **WHEN** fallback 工具链调用 MCP 适配工具
- **THEN** 系统 MUST 向配置的 MCP endpoint 发送 JSON-RPC tool call，并把文本结果返回给现有工具链

#### Scenario: MCP 工具调用失败时返回包装错误
- **WHEN** MCP endpoint 返回错误、超时或无效响应
- **THEN** 系统 MUST 返回带工具名的包装错误，让 fallback 工具链按既有错误语义处理

### Requirement: Gateway 示例展示嵌入式 Chat Completions
系统 SHALL 提供轻量 Gateway 示例，展示如何在已有 Go HTTP 服务中暴露 OpenAI-compatible chat completions 路径。

#### Scenario: Gateway 示例调用 SDK Session
- **WHEN** 示例 handler 收到一个非流式 chat completions 请求
- **THEN** 示例 MUST 从请求中提取 user/session 和最后一条用户消息，调用 SDK Session，并返回 OpenAI-compatible 响应形状

#### Scenario: Gateway 示例保持 SDK 定位
- **WHEN** 用户阅读 Gateway 示例或 README
- **THEN** 文档 MUST 明确该示例不是内置控制台、不是用户系统、不是完整 SaaS control plane
