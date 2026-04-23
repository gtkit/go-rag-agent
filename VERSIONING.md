# Versioning

## 版本策略

本仓库从现在开始采用语义化版本：

- `MAJOR.MINOR.PATCH`
- 示例：`v0.1.0`

含义：
- `MAJOR`
  说明：出现不兼容 API 变更时递增
- `MINOR`
  说明：向后兼容的新能力时递增
- `PATCH`
  说明：向后兼容的修复、文档或实现细节优化时递增

## 当前阶段建议

在 API 还持续演进时，建议保持 `v0.x.y`：

- `v0.1.0`
  说明：第一个可发布基线版本
- `v0.2.0`
  说明：新增较大能力但不破坏兼容
- `v0.2.1`
  说明：问题修复或文档补充

只有在你确认以下条件长期稳定后，再考虑进入 `v1.0.0`：
- 根包 API 基本稳定
- 配置项不会频繁重命名
- README / CHANGELOG / 示例已完整
- 主要检索、导入、观测、降级路径都经过稳定验证

## API 稳定性分层

当前仓库把对外承诺分成 3 层：

- `Stable Core`
  说明：README 快速开始和 `examples/basic` / `examples/service` / `examples/pgvector` 覆盖的核心接入面，包括 `Config` 基础字段、`New`、`AddKnowledge`、`GetSession`、`Ask` / `AskWithOptions` / `AskStream`、`Answer.Citations`、`RunEvalSuite`、`WriteEvalReportJSON` / `ReadEvalReportJSON`、`NewPGVectorStore`。
  规则：这一层出现不兼容变化时，必须同步更新 `CHANGELOG.md` 和迁移说明；如果变化频率仍然偏高，就不应该进入 `v1.0.0`。
- `Advanced Integration`
  说明：`RuntimeComponents`、`RetrievalComponents`、`StorageComponents`、`MemoryComponents`、`ToolRegistry`、`AccessBoundary`、`ProviderGovernance` 等高级接线面。
  规则：允许在 `v0.x` 继续补字段和文档，但不应无提示删除已文档化能力。
- `Experimental Assets`
  说明：`PDFOCRBridge`、`ImageTextBridge`、trace summary 基线资产、仓库内 `cmd/` 辅助工具，以及演示性文档流程。
  规则：优先快速演进；如果输出结构或用法变化，至少在 README 或 CHANGELOG 中说明。

## Tag 规则

从现在开始，推荐优先使用语义化 tag：

- `v0.1.0`
- `v0.1.1`
- `v0.2.0`

历史上已有的描述性 tag 可以保留，但后续不再作为主发布规则。

推荐：
- 正式发布：只打语义化 tag
- 临时里程碑：如确有需要，可额外打描述性 tag，但不替代正式版本 tag

## 发布检查清单

每次正式发布前至少执行：

```bash
go test ./... -count=1
go vet ./...
go test -race -count=1 -timeout=5m ./...
go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...
```

如果本次涉及检索增强，还建议执行：

```bash
go test -bench=. -run '^$' ./internal/retrieval ./internal/storage
```

如果本次涉及定位资产或评测基线，还建议执行：

```bash
go run ./cmd/generate-sdk-positioning-baseline
```

## CHANGELOG 规则

每次准备发布前：
- 先更新 `CHANGELOG.md`
- 再提交 release commit
- 最后打语义化 tag

推荐顺序：
1. 更新 `CHANGELOG.md`
2. 提交
3. 打 tag
4. 推送 tag
