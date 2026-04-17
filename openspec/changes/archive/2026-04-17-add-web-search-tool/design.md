## Context

当前问答链路是“本地知识库优先”的：
- 本地检索成功时，直接把 evidence 交给模型
- 本地检索证据不足时，返回 `ErrEvidenceInsufficient`

这让库在企业内知识库场景很稳，但也意味着它还不能在必要时去联网补充信息。当前内部已经有 ReAct runner 和工具调用基础，因此最自然的扩展点是增加一个 web search 工具，并在“本地证据不足”时放行到该路径。

## Goals / Non-Goals

**Goals：**

- 保持“本地知识库优先”
- 仅在本地证据不足时才允许联网搜索
- 首个 provider 使用 Tavily Search API
- 使用 `github.com/gtkit/httpc` 封装外部 HTTP JSON 调用

**Non-Goals：**

- 不一次性接多个 web search provider
- 不接浏览器自动抓取或通用网页解析
- 不在本地证据充足时主动混入联网搜索结果
- 不实现完整的远程结果 citation 结构重建

## Decisions

### 1. 首版 provider 固定为 Tavily

选择：首版只支持 Tavily，并通过配置启用。

原因：
- Tavily 是标准 JSON API，适合快速稳定落地
- 比同时支持多个 provider 更容易先形成生产级闭环

### 2. 外部 HTTP 调用统一使用 `github.com/gtkit/httpc`

选择：在 `internal/websearch` 中用 `httpc.Client.RequestJSON(...)` 调 Tavily。

原因：
- 该包已明确提供生产级 JSON HTTP client 能力
- 与当前 Go 企业级要求更一致，便于统一超时、日志和上下文传递

### 3. 本地证据不足时放行到 ReAct + web search tool

选择：
- 本地检索成功：继续走现有 evidence-first 模型路径
- 本地检索返回 `ErrEvidenceInsufficient` 且启用联网搜索：不直接报错，改为进入带工具的 ReAct 路径

原因：
- 保持本地优先
- 只有确实缺本地证据时才给模型联网能力

### 4. 首版不重构 `Answer.Citations`

选择：联网搜索结果先作为工具文本上下文交给模型，不把远程结果塞进 `Answer.Citations`

原因：
- 当前 `Citation` 结构是本地 chunk 语义，直接塞远程结果会污染契约
- 首版先把联网能力打通，后续再考虑 remote citation 抽象

## Risks / Trade-offs

- [模型在工具路径上不稳定] → 仅在本地证据不足时才使用，降低不确定性
- [远程结果没有结构化 citations] → 首版接受这个限制，并在 README 说明
- [外部搜索 API 超时/错误] → 沿用当前超时控制，并把失败保持为工具错误

## Migration Plan

1. 增加 web search 配置与校验
2. 实现 Tavily client 和 web search tool
3. 修改问答流程，在本地证据不足且启用配置时进入工具路径
4. 更新 README

## Open Questions

无
