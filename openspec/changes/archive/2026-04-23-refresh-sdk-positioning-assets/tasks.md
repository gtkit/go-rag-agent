## 1. 设计与工作流

- [x] 1.1 创建定位刷新设计文档
- [x] 1.2 初始化 OpenSpec change 并补齐 proposal / design / tasks / spec

## 2. 基线生成能力

- [x] 2.1 为 baseline 生成逻辑补测试
- [x] 2.2 实现 baseline 生成逻辑与命令入口
- [x] 2.3 生成并提交样例结果文件

## 3. 文档与示例

- [x] 3.1 重写 README 顶部定位与场景导航
- [x] 3.2 增加服务内嵌和 pgvector 示例
- [x] 3.3 补 API 稳定性分层与 baseline 文档
- [x] 3.4 更新 VERSIONING / CHANGELOG

## 4. Release Gate

- [x] 4.1 增加 `scripts/verify.sh`
- [x] 4.2 增加 GitHub Actions CI workflow

## 5. 验证

- [x] 5.1 运行 `openspec validate`
- [x] 5.2 运行 `go vet ./...`
- [x] 5.3 运行 `golangci-lint run ./...`
- [x] 5.4 运行 `go test -race -count=1 -timeout=5m ./...`
- [x] 5.5 运行 `go run ./cmd/generate-sdk-positioning-baseline`
