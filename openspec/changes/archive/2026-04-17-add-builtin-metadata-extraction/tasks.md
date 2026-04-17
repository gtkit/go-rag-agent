## 1. OpenSpec 与测试基线

- [x] 1.1 为内置元数据提取补齐 proposal、design、delta spec
- [x] 1.2 先补失败测试，覆盖 sidecar metadata、front matter 提取与 metadata 合并

## 2. 核心实现

- [x] 2.1 在 source 层实现 sidecar metadata 自动发现、解析与 sidecar 文件跳过逻辑
- [x] 2.2 在 loader 层实现 Markdown front matter 提取、正文清洗与 metadata 合并
- [x] 2.3 确认导入后的 chunk metadata 能直接服务现有 Metadata 过滤检索

## 3. 文档与验证

- [x] 3.1 更新中文 README，补充 front matter 和 sidecar metadata 用法
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-builtin-metadata-extraction --type change --strict --json --no-interactive`
