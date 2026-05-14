## 1. OpenSpec

- [x] 1.1 验证 proposal/design/specs/tasks apply-ready
- [x] 1.2 运行 strict validation

## 2. 测试先行

- [x] 2.1 为 live provider env parsing 和 skip reason 写失败测试
- [x] 2.2 为部署模板内容和敏感信息边界写失败测试
- [x] 2.3 为 README/生产文档命令矩阵写失败测试

## 3. 实现

- [x] 3.1 增加 live provider integration test helper 和 opt-in 测试
- [x] 3.2 增加 `.env.production.example`
- [x] 3.3 增加 `deploy/compose/pgvector.compose.yml`
- [x] 3.4 增加 `docs/production.md`
- [x] 3.5 更新 README 生产验证入口

## 4. 验证

- [x] 4.1 运行 targeted tests
- [x] 4.2 运行 `openspec validate add-production-validation-assets --type change --strict --json --no-interactive`
- [x] 4.3 运行 `golangci-lint run ./...`
- [x] 4.4 运行 `go vet ./...`
- [x] 4.5 运行 `go test -count=1 -cover ./...`
- [x] 4.6 运行 `go test -race -count=1 -timeout=5m ./...`
- [x] 4.7 运行敏感信息和架构边界扫描
