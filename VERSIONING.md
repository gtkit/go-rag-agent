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
