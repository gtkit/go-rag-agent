## Context

当前检索链路是：
- query 重写
- query embedding
- `store.SearchWithFilter(...)`
- evidence context assembly

这个链路在语义相关场景表现稳定，但对精确词项、缩写、接口名、文件名这样的 lexical 信号利用不足。与此同时，仓库现有能力已经足够成熟：
- 导入侧有 OCR 和 metadata
- 检索侧已有 source path / metadata 过滤

因此现在适合在“同一条根检索链路”上升级检索质量，而不是继续扩更多导入来源。

## Goals / Non-Goals

**Goals：**

- 启用 `EnableHybridSearch`
- 让 hybrid retrieval 同时服务同步问答、流式问答和 retrieval tool 路径
- 启用 `EnableRerank`，对 hybrid shortlist 做可选重排
- 保持现有公开调用 API 兼容

**Non-Goals：**

- 不引入新的外部 rerank 模型或服务
- 不实现复杂倒排索引或独立全文检索后端
- 不实现多语言高级分词、stemming 或 BM25 全量体系

## Decisions

### 1. lexical recall 基于当前候选全集扫描

选择：复用现有 `SearchWithFilter(...)` 返回的候选文档文本，对候选全集执行本地 lexical 打分，而不是增加新的存储接口。

原因：
- 最小改动，不破坏现有 `VectorStore` 抽象
- 当前 `chromem` 后端已经能返回带正文和 metadata 的 `SearchHit`
- 先保证行为正确，再考虑更重的全文索引

### 2. hybrid 融合采用 RRF

选择：用 Reciprocal Rank Fusion 融合 vector ranking 与 lexical ranking。

原因：
- 不需要两种分值天然同量纲
- 实现简单，排序稳定
- 很适合第一版 hybrid

### 3. rerank 首版采用本地规则实现

选择：`EnableRerank` 开启时，只对 hybrid shortlist 做本地规则重排，结合 lexical 命中强度和 fused rank。

原因：
- 不引入新的外部模型依赖
- 把 rerank 先做成内部可插拔阶段，后续再替换成更强实现

### 4. tool 路径与主问答路径必须共用同一检索器

选择：root retriever 内统一处理 vector-only / hybrid / rerank 三种模式。

原因：
- 避免 `Ask` 与 tool 路径行为分叉
- 现有 rootRetriever 已是共同入口，最适合作为升级点

## Risks / Trade-offs

- [hybrid 首版扫描候选全集，代价高于纯向量] → 仅在 `EnableHybridSearch` 打开时启用；关闭时保持现有快路径
- [lexical tokenizer 过于简单] → 首版只做稳定、可测的简单 tokenizer，后续再增强
- [本地 rerank 不如外部模型强] → 先提供可选排序增强入口，后续再替换实现

## Migration Plan

1. 补配置和检索行为测试
2. 新增 `internal/retrieval`，实现 lexical recall、RRF 融合和 shortlist rerank
3. 接入 `agent.go` root retriever
4. 更新 README 和 OpenSpec

## Open Questions

无
