## 1. OpenSpec 与测试基线

- [x] 1.1 为 hybrid retrieval + optional rerank 补齐 proposal、design、delta spec
- [x] 1.2 先补失败测试，覆盖配置开关、hybrid 融合、流式路径和 optional rerank

## 2. 核心实现

- [x] 2.1 新增 `internal/retrieval`，实现 lexical recall、RRF 融合和 shortlist rerank
- [x] 2.2 在 root retriever 中接入 `EnableHybridSearch` 与 `EnableRerank`
- [x] 2.3 保证同步问答、流式问答和 retrieval tool 路径统一走同一检索策略

## 3. 文档与验证

- [x] 3.1 更新中文 README，说明 hybrid / rerank 开关、行为和限制
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-hybrid-retrieval-rerank --type change --strict --json --no-interactive`
