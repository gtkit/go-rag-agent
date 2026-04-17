# Changelog

本文件记录 `go-rag-agent` 的版本变化。

格式约定：
- `Added`：新增能力
- `Changed`：已有能力行为调整
- `Fixed`：缺陷修复

## [Unreleased]

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

### Changed
- `EnableHybridSearch` 从“被拒绝”调整为可实际启用
- `EnableRerank` 从“被拒绝”调整为可在 hybrid 开启时启用
- README 增加企业级调参和观测说明

## [2026-04-17]

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
