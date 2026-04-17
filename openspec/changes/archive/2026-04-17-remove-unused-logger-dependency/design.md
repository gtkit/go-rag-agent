## Context

仓库当前声明了 `github.com/gtkit/logger v1.6.2` 直接依赖，但代码中没有实际调用它。`go mod why` 显示该依赖只因为 `bootstrap_deps_test.go` 的空白导入被保留。与此同时：

- `go list -m -versions github.com/gtkit/logger` 可以通过默认代理看到 `v1.6.2`
- `GOPROXY=direct go list -m -versions github.com/gtkit/logger` 直连源端时最高只到 `v1.4.6`

这说明 `v1.6.2` 对当前仓库来说既未使用，又存在源端可见性不一致问题。

## Goals / Non-Goals

**Goals:**

- 彻底移除未使用的 `github.com/gtkit/logger v1.6.2`
- 让仓库不再依赖 proxy 才能解析的该版本
- 保持现有库行为和公开 API 不变

**Non-Goals:**

- 不把 `logger` 升级到 `v2`
- 不引入新的日志抽象或日志实现
- 不修改任何业务逻辑

## Decisions

### 1. 直接删除依赖，而不是升级到 v2

选择：删除 `github.com/gtkit/logger` 依赖与空白导入，不引入 `github.com/gtkit/logger/v2`。

原因：
- 当前仓库没有实际使用 logger 包
- 为“未使用依赖”升级主版本没有意义，只会继续保留无必要耦合

备选方案：
- 升级到 `v2`：不能解决“未使用依赖”根问题

### 2. 保留 bootstrap 依赖锚点文件，但只删除 logger 空白导入

选择：最小化修改 `bootstrap_deps_test.go`，只删掉 logger 对应的空白导入。

原因：
- 该文件仍然承担其他测试期依赖锚点作用
- 改动更小、风险更低

## Risks / Trade-offs

- [某个隐藏路径其实依赖 logger] → 用 `go mod why` 和全量测试验证；当前证据显示只有测试空白导入使用它
- [go mod tidy 带来无关依赖变化] → 只接受与 logger 相关的依赖图清理结果

## Migration Plan

1. 删除 `bootstrap_deps_test.go` 中的 logger 空白导入
2. 清理 `go.mod` / `go.sum`
3. 运行测试与 lint 验证

## Open Questions

无
