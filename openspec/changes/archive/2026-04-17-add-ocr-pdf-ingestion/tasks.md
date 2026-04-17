## 1. OpenSpec 与测试基线

- [x] 1.1 为 OCR / 扫描版 PDF 导入补齐 proposal、design、delta spec
- [x] 1.2 先补失败测试，覆盖文本型 PDF 快路径、扫描版 PDF OCR fallback 和缺少 OCR 配置错误

## 2. 核心实现

- [x] 2.1 新增 OCR bridge 配置类型、默认值与校验逻辑
- [x] 2.2 在 PDF loader 中接入“直接文本提取优先，OCR fallback 其次”的流程
- [x] 2.3 让 OCR bridge 输出文本后复用现有分块、embedding 和存储链路

## 3. 文档与验证

- [x] 3.1 更新中文 README，补充 OCR 使用方式、依赖前提和限制说明
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-ocr-pdf-ingestion --type change --strict --json --no-interactive`
