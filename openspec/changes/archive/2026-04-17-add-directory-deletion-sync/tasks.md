## 1. OpenSpec 与测试基线

- [x] 1.1 为目录删除同步补齐变更 proposal、design 和 delta spec
- [x] 1.2 先补回归测试，覆盖目录重导入时删除已移除文件、空目录清理和持久化模式跨重启场景

## 2. 核心实现

- [x] 2.1 新增目录导入清单状态组件，支持内存模式和 `DataDir` 持久化模式
- [x] 2.2 扩展向量存储契约与 chromem 实现，支持按 `source path` 删除 chunk
- [x] 2.3 修改 `AddKnowledge(ctx, DirSource(path))`，在成功导入后执行 stale source path 清理并提交目录清单

## 3. 说明与验证

- [x] 3.1 更新中文 README 与必要注释，说明 `DirSource(path)` 的删除感知同步语义
- [x] 3.2 运行 `go test ./... -count=1`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`、`golangci-lint run ./...`、`openspec validate add-directory-deletion-sync --type change --strict --json --no-interactive`
