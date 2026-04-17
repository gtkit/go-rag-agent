## 1. OpenSpec 与测试基线

- [x] 1.1 为 hybrid tuning + benchmarks 补齐 proposal、design、delta spec
- [x] 1.2 先补失败测试，覆盖参数默认值、配置校验和调优行为

## 2. 核心实现

- [x] 2.1 为 hybrid / rerank 增加关键调优参数与默认值
- [x] 2.2 让检索增强逻辑读取这些参数，而不是依赖硬编码常量
- [x] 2.3 新增 benchmark，对比 vector-only、hybrid、hybrid+r​​erank

## 3. 文档与验证

- [x] 3.1 更新中文 README，补充默认值、调参建议和 benchmark 运行方式
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-hybrid-tuning-benchmarks --type change --strict --json --no-interactive`
