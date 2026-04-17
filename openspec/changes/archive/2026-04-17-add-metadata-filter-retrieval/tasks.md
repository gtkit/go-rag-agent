## 1. OpenSpec 与测试基线

- [x] 1.1 为元数据过滤检索补齐 proposal、design、delta spec
- [x] 1.2 先补失败测试，覆盖存储过滤检索、同步问答过滤和流式问答过滤

## 2. 核心实现

- [x] 2.1 新增公开查询选项与检索过滤类型，保持现有 `Ask` / `AskStream` 兼容
- [x] 2.2 扩展存储检索契约与 chromem 过滤逻辑，支持 `source path`、前缀和元数据过滤
- [x] 2.3 把过滤条件接入同步问答与流式问答链路

## 3. 文档与验证

- [x] 3.1 更新中文 README，补充过滤检索用法和限制说明
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-metadata-filter-retrieval --type change --strict --json --no-interactive`
