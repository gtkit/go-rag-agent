## Context

当前实现已经具备：
- `EnableHybridSearch`
- `EnableRerank`
- lexical recall
- RRF 融合
- 本地 shortlist rerank

但还有三个明显短板：
- RRF 常量硬编码
- hybrid 候选规模硬编码
- rerank shortlist 大小硬编码/隐式

这会让线上调优只能靠改代码，无法通过配置进行容量和延迟折中。同时，虽然已有检索增强能力，但还缺少 benchmark 作为性能基线。

## Goals / Non-Goals

**Goals：**

- 给 hybrid / rerank 增加少量但关键的调优参数
- 保持默认值保守、稳定，可直接上线
- 增加 benchmark，覆盖 vector-only、hybrid、hybrid+r​​erank
- README 提供默认值建议和调参思路

**Non-Goals：**

- 不实现自动参数搜索
- 不接入线上指标系统或外部 profiling 基础设施
- 不改变现有公开问答 API

## Decisions

### 1. 只暴露三类关键参数

选择：
- `HybridCandidateMultiplier`
- `HybridRRFK`
- `RerankShortlistMultiplier`

原因：
- 这三项足以控制质量/延迟的主要平衡点
- 比暴露大量内部细节参数更适合企业级默认配置

### 2. 默认值保持保守

建议默认：
- `HybridCandidateMultiplier = 4`
- `HybridRRFK = 60`
- `RerankShortlistMultiplier = 2`

原因：
- 与当前实现行为接近，迁移风险低
- 足够适合作为首版线上默认值

### 3. benchmark 放在 `internal/retrieval`

选择：把检索增强相关 benchmark 主要放在 `internal/retrieval/bench_test.go`。

原因：
- 便于隔离比较 vector-only、hybrid、rerank 本身的额外成本
- 不依赖外部模型或完整 Agent 初始化

## Risks / Trade-offs

- [参数过多让配置复杂化] → 只暴露三项关键参数，并提供 README 推荐值
- [benchmark 过于理想化] → 保持基准场景稳定且可重复，README 明确它是相对比较基线
- [参数值不合适导致线上延迟升高] → 保守默认值 + 文档化调参建议

## Migration Plan

1. 新增配置字段与测试
2. 让 hybrid/rerank 读取这些参数
3. 新增 benchmark
4. 更新 README

## Open Questions

无
