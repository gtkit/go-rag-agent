## Context

库当前已经具备：
- 本地 `.txt` / `.md` / 文本型 `.pdf` 导入
- 有道笔记桥接导入
- 目录删除感知同步
- Session 级同步问答与流式问答

但检索阶段仍是“全库查询”。对于个人知识库，调用方经常需要：
- 只查某个目录
- 只查某个文件
- 只查带某个元数据标签的文档

现有 `Ask` / `AskStream` 签名很轻，不适合直接塞更多位置参数；同时需要保持 Phase 1 已有 API 兼容。

## Goals / Non-Goals

**Goals：**

- 为单次问答增加可选的检索过滤条件
- 支持三类过滤：精确 `source path`、`source path` 前缀、精确元数据匹配
- 保持现有 `Ask` / `AskStream` 行为与签名不变
- 更新 README，给出中文使用方式

**Non-Goals：**

- 不实现布尔表达式、范围查询、模糊匹配或 DSL
- 不引入新的索引后端或倒排索引
- 不改变知识导入数据结构，只复用现有 `source path` 与 `Metadata`

## Decisions

### 1. 用 `QueryOptions` 承载可选检索过滤条件

选择：
- 保留 `Ask(ctx, query)` 和 `AskStream(ctx, query, emit)` 原样不动
- 新增 `AskWithOptions(ctx, query, opts)` 和 `AskStreamWithOptions(ctx, query, opts, emit)`
- `QueryOptions` 内含 `RetrievalFilter`

原因：
- 兼容现有调用方
- 给后续更多查询级选项预留位置，不把方法签名继续拉长

备选方案：
- 直接改 `Ask` / `AskStream` 签名：破坏兼容
- 给 Session 挂可变过滤状态：会引入额外状态管理和并发语义

### 2. 过滤能力只做“精确值 + 路径前缀”

选择：
- `SourcePaths []string`
- `SourcePrefixes []string`
- `Metadata map[string]string`

原因：
- 已覆盖“某文件 / 某目录 / 某标签”的主流使用场景
- 与当前存储中已有字段天然对齐

备选方案：
- 做完整表达式语言：明显超出当前阶段范围
- 只做 `Metadata`：无法直接覆盖目录范围检索

### 3. 存储层用统一过滤结构，chromem 先做正确性优先实现

选择：
- 在 `internal/storage` 增加过滤结构，并把 `Search` 扩展为接收过滤条件
- `ChromemStore.Search` 在存在过滤条件时允许扩大候选集，再在本地做过滤与截断

原因：
- chromem 的原生 metadata 过滤适合精确匹配，不适合目录前缀
- 先保证过滤结果正确，再考虑性能优化

备选方案：
- 强行只用底层原生过滤：目录前缀场景做不完整
- 在根包层对 topK 结果再过滤：会漏掉真正应该命中的 filtered hit

### 4. 过滤条件只作用于当前问答的检索阶段

选择：
- 本次改动只保证根问答检索遵守过滤条件
- 不把过滤条件提升成 Agent 全局配置

原因：
- 过滤需求天然是“单次查询上下文”
- 避免把 Session 或 Agent 变成带可变检索范围的状态对象

## Risks / Trade-offs

- [过滤检索会扩大候选集，带来额外开销] → 只在有过滤条件时走扩大候选集路径；无过滤保持当前快路径
- [公开 API 增长] → 用 `QueryOptions` 收口，避免散落多个新方法参数
- [用户误解 metadata 为模糊匹配] → README 明确说明当前是精确匹配

## Migration Plan

1. 新增 OpenSpec 变更与回归测试
2. 扩展存储检索契约和 chromem 过滤逻辑
3. 新增公开查询选项类型和带 options 的 Session 方法
4. 更新 README

回滚策略：
- 回滚代码即可恢复现有无过滤检索行为
- 无额外持久化数据迁移

## Open Questions

无
