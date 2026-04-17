## 1. OpenSpec 与测试基线

- [x] 1.1 为 retrieval observability + fallbacks 补齐 proposal、design、delta spec
- [x] 1.2 先补失败测试，覆盖详细 metrics 回调与 hybrid/rerank fallback 行为

## 2. 核心实现

- [x] 2.1 新增可选观测回调接口与事件结构，保持现有 `Callback` 接口兼容
- [x] 2.2 为检索与模型路径补充 metrics 发出
- [x] 2.3 为 hybrid 和 rerank 路径补充阶段性 fallback 与事件通知

## 3. 文档与验证

- [x] 3.1 更新中文 README，补充企业级观测点和降级语义
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`、`openspec validate add-retrieval-observability-and-fallbacks --type change --strict --json --no-interactive`
