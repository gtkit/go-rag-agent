# Changelog

本文件记录 `go-rag-agent` 的版本变化。

格式约定：
- `Added`：新增能力
- `Changed`：已有能力行为调整
- `Fixed`：缺陷修复

## [Unreleased]

## [v0.5.0] - 2026-04-21

### Added
- 长期记忆分层能力：
  - `MemoryComponents`
  - `LongTermMemoryStore`
  - `NewInMemoryLongTermMemoryStore()`
  - `LongTermMemoryTopK`
  - `LongTermMemoryThreshold`
  - `MaxMemoryTokens`
- 同 Session 的长期语义记忆回忆：
  - 成功问答后写入长期记忆
  - 后续问答前检索并注入 `Relevant long-term memory`

### Changed
- `Ask` / `AskStream` 现在会在成功完成后把当前轮次写入长期记忆
- README 补充短期/长期记忆分层和默认 in-memory 语义

### Fixed
- 修复长期记忆接入后 prompt 构造与现有 graph 请求的兼容性问题

## [v0.4.0] - 2026-04-21

### Added
- 上下文窗口治理能力：
  - `MaxPromptTokens`
  - `MaxHistoryTokens`
  - `MaxEvidenceTokens`
  - `MaxSummaryTokens`
- 整轮执行时长预算：
  - `MaxExecutionDuration`
- 基础 prompt hardening：
  - `EnablePromptHardening`
  - 对检索证据与工具回灌文本做不可信上下文包裹与明显注入模式过滤
- 本地历史摘要压缩：
  - 超出历史预算时生成 `Conversation summary`

### Changed
- prompt 构造现在会按 token 预算裁剪历史与证据
- 同步与流式问答都接入整轮执行时长预算
- README 补充上下文治理与提示硬化配置说明

### Fixed
- 修复 prompt 预算引入后对默认 graph 请求零值预算的兼容性问题

## [v0.3.0] - 2026-04-21

### Added
- 运行时扩展边界：
  - `RuntimeComponents`
  - `StorageComponents`
  - 可注入 `VectorStore` / `DocumentLoader` / `Reranker`
- 单次执行结构化 trace：
  - `ExecutionTrace`
  - `TraceRecorder`
  - `Answer.Trace`
  - `StreamEvent.Trace`
- PostgreSQL / pgvector 第二实现：
  - `NewPGVectorStore(...)`
  - `PGVectorStoreConfig`
  - embedded mode 与 server mode 双模式说明
- 工具注册表与调用上限：
  - `Tool`
  - `ToolRegistry`
  - `MaxToolCalls` 真实生效
- 同步结构化输出能力：
  - `AskStructured`
  - `AskStructuredWithOptions`
  - `StructuredAnswer`

### Changed
- 默认存储、文档加载与规则重排从内部实现提升为默认 adapter
- 默认工具接线改为通过统一注册表构建
- 自定义同名 `search_web` 工具现在可覆写默认 web 工具
- README 补充 runtime 注入、存储边界、pgvector、工具注册表和结构化输出说明

### Fixed
- 修复工具调用上限引入后对手工构造 `Agent` 场景的兼容性回归
- 修复 pgvector 初始化过程中的扩展注册竞态与真实连库集成问题

## [v0.2.0] - 2026-04-17

### Added
- Hybrid retrieval（向量召回 + lexical recall）与可选 rerank
- hybrid/rerank 调优参数：
  - `HybridCandidateMultiplier`
  - `HybridRRFK`
  - `RerankShortlistMultiplier`
- hybrid / rerank benchmark
- 详细运行时观测回调：
  - `RetrievalMetricsCallback`
  - `ModelMetricsCallback`
  - `FallbackCallback`
- 运行时降级事件：
  - hybrid 失败退回 `vector_only`
  - rerank 失败退回 `hybrid`
- 可选联网搜索能力：
  - `EnableWebSearch`
  - `WebSearchConfig`
  - Tavily provider
  - `search_web` tool
- 新增 `runtime-observability` 与 `web-search-tool` 主 specs
- 发布文档：
  - `VERSIONING.md`
  - 企业级 release/tag 规则

### Changed
- `EnableHybridSearch` 从“被拒绝”调整为可实际启用
- `EnableRerank` 从“被拒绝”调整为可在 hybrid 开启时启用
- 模块路径切换为 `github.com/gtkit/go-rag-agent`
- README 增加企业级调参和观测说明
- README 增加联网搜索启用方式和限制

## [v0.1.0] - 2026-04-17

### Added
- 本地 `.txt` / `.md` / `.pdf` 导入
- 扫描版 PDF OCR bridge 导入
- 有道笔记桥接导入
- 目录删除感知同步
- 内置 metadata 提取：
  - Markdown YAML front matter
  - sidecar metadata
- `AskWithOptions` / `AskStreamWithOptions` 检索过滤

### Fixed
- 清理未使用的 `github.com/gtkit/logger v1.6.2`
