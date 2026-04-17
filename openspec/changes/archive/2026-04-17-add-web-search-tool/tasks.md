## 1. OpenSpec 与测试基线

- [x] 1.1 为 web search tool 补齐 proposal、design、spec、tasks
- [x] 1.2 先补失败测试，覆盖配置校验、Tavily client、web search tool 和本地证据不足时的放行行为

## 2. 核心实现

- [x] 2.1 增加联网搜索配置与校验逻辑
- [x] 2.2 使用 `github.com/gtkit/httpc` 实现 Tavily client
- [x] 2.3 实现 web search tool，并在本地证据不足时接入问答流程

## 3. 文档与验证

- [x] 3.1 更新中文 README，说明联网搜索启用方式和限制
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-web-search-tool --type change --strict --json --no-interactive`
