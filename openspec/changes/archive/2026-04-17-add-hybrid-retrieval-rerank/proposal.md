## Why

当前库已经具备本地导入、目录同步、OCR fallback、内置 metadata 提取和过滤检索，但检索阶段仍然主要依赖单一路径的向量召回。这会在精确关键词、短术语和结构化文档名场景下错过本应命中的证据，因此下一步最值得优先做的是 hybrid retrieval，并在此基础上提供可选 rerank。

## What Changes

- 启用 `EnableHybridSearch`，把检索从纯向量召回升级为向量召回 + lexical recall 融合。
- 启用 `EnableRerank`，对 hybrid shortlist 执行可选的本地 rerank。
- 保持现有 `Ask` / `AskWithOptions` / `AskStreamWithOptions` API 不变，只改变内部检索行为。
- README 更新中文说明，补充 hybrid / rerank 开关、行为和限制。

## Capabilities

### New Capabilities

无

### Modified Capabilities

- `embedded-rag-library`: 修改同步问答和流式问答的检索行为，使其支持 hybrid retrieval 和可选 rerank。

## Impact

- 受影响代码：`config.go`、`agent.go`、`internal/storage/*`、新增 `internal/retrieval/*`、相关测试与 `README.md`
- 受影响行为：开启开关后，检索候选和证据排序将不再是纯向量排序
- 不新增外部服务依赖，rerank 首版使用本地规则实现
