## Why

当前仓库把 `github.com/gtkit/logger v1.6.2` 作为直接依赖保留在 `go.mod` 中，但代码里并没有真正使用它，只剩一个测试期空白导入锚点。与此同时，直连源端查询时 `v1.6.2` 已不可见，只在 proxy 结果里还能解析，这会让依赖解析行为变得不稳定。

## What Changes

- 删除 `github.com/gtkit/logger v1.6.2` 的直接依赖
- 删除仅用于保留该依赖的测试空白导入锚点
- 运行 `go mod tidy` 清理 `go.sum`
- 不引入 `github.com/gtkit/logger/v2`

## Capabilities

### New Capabilities

- `build-dependency-hygiene`: 约束库仓库不应保留未使用且来源不稳定的直接依赖

### Modified Capabilities

无

## Impact

- 受影响文件：`go.mod`、`go.sum`、`bootstrap_deps_test.go`
- 受影响行为：构建依赖图更干净，避免继续依赖仅在 proxy 可见的 `v1.6.2`
- 不影响库的运行时 API 和行为
