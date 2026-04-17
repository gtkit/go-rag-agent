## 1. OpenSpec 与依赖确认

- [x] 1.1 补齐 proposal、design、spec、tasks，记录删除未使用 logger 依赖的原因
- [x] 1.2 确认 `github.com/gtkit/logger` 仅由测试空白导入保留，不存在实际代码引用

## 2. 依赖清理

- [x] 2.1 删除 `bootstrap_deps_test.go` 中的 logger 空白导入
- [x] 2.2 运行 `go mod tidy`，清理 `go.mod` 与 `go.sum` 中的 logger v1.6.2

## 3. 验证

- [x] 3.1 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`
- [x] 3.2 运行 `openspec validate remove-unused-logger-dependency --type change --strict --json --no-interactive`
