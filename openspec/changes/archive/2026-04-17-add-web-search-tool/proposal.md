## Why

当前库已经能优先回答本地知识库内容，但在本地证据不足时只会返回 `ErrEvidenceInsufficient`，无法进一步利用联网搜索补充上下文。为了让大模型在“本地优先”的前提下具备联网能力，需要增加一个可选的 web search 路径。

## What Changes

- 新增可选的联网搜索配置与 Tavily provider。
- 使用 `github.com/gtkit/httpc` 作为外部 HTTP JSON client。
- 当本地检索证据不足且启用了联网搜索时，允许问答流程进入 web search 工具路径，而不是直接返回证据不足。
- README 更新中文说明，补充联网搜索启用方式和限制。

## Capabilities

### New Capabilities

- `web-search-tool`: 提供可选的外部联网搜索能力，首个 provider 为 Tavily

### Modified Capabilities

- `embedded-rag-library`: 修改问答行为，使本地证据不足时可以在启用配置后进入联网搜索路径

## Impact

- 受影响代码：`config.go`、`agent.go`、`internal/tools/*`、新增 `internal/websearch/*`、相关测试与 `README.md`
- 受影响行为：本地知识库仍优先；仅在本地证据不足时才允许联网搜索介入
- 新增依赖：`github.com/gtkit/httpc v1.0.0`
