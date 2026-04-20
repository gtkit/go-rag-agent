## Context

目前这个库已经把聊天模型和 embedding 暴露为根包可注入 runtime，但 `VectorStore`、文件加载和 rerank 仍藏在 `internal/storage`、`internal/rag`、`internal/retrieval` 中。默认行为虽然稳定，但调用方无法在不依赖 `internal/*` 的前提下替换这些组件，也就无法把这套库真正作为可扩展的 RAG 内核来使用。

本 change 的目的不是增加更多后端，而是先把三块边界稳定公开，并保证当前默认路径完全不变。

## Goals / Non-Goals

**Goals:**
- 在根包公开稳定的 `VectorStore`、`DocumentLoader`、`Reranker` 接口
- 在根包公开这些接口配套的数据类型和默认 adapter 构造器
- 为 `Config` 增加存储/加载/rerank 组件注入入口
- 让 `Agent` 通过接口编排，同时保持默认行为与问答 API 不变

**Non-Goals:**
- 不引入第二套 `VectorStore`、`DocumentLoader` 或 `Reranker` 实现
- 不增加基于字符串的 registry/factory
- 不在这轮公开 `Retriever` 抽象
- 不改变现有知识导入、问答、trace 或流式协议

## Decisions

### 1. 采用薄抽象层，而不是一次性做完整插件化

根包只新增稳定接口、数据类型和默认构造器；当前 `ChromemStore`、文件加载逻辑和规则 rerank 通过默认 adapter 接入。这样做的原因是当前最需要解决的问题是“边界不可替换”，不是“后端切换方式不够花哨”。

备选方案：
- 一次性做 registry/factory。否决原因：会放大配置面和复杂度。
- 顺手公开 `Retriever`。否决原因：会把 hybrid、lexical 和 context assembly 一起卷入，超出当前 change 范围。

### 2. `Agent` 继续做编排层，不把内部实现对象外露

`AddKnowledge` 和问答路径只改为依赖根包公开接口：
- 导入：`KnowledgeSource.Resolve -> DocumentLoader -> Chunker -> Embedder -> VectorStore.Upsert`
- 问答：`Embed -> VectorStore.SearchWithFilter -> lexical/hybrid merge -> optional Reranker -> context assembly -> model`

这样可以让调用方替换组件，而不需要理解内部包结构。

### 3. 新增独立的 `StorageComponents` 注入结构

这组注入与现有 `RuntimeComponents` 并列存在，不混在一起：
- `VectorStore`
- `DocumentLoader`
- `Reranker`

默认不传就使用默认 adapter；传入某一项时只覆盖该项，其余部分仍使用默认路径。

### 4. 默认行为必须通过现有测试锁死

这轮抽象如果让 OCR、front matter、sidecar metadata、hybrid、rerank、web fallback 出现行为回归，就说明边界拆分方式有问题。因此本 change 直接把“默认兼容性不回归”作为核心验收条件。

## Risks / Trade-offs

- [根包类型增加，API 面扩大] → 仅公开当前需要稳定的最小集合，不把 `internal/*` 全量搬出
- [内部与公开类型转换带来维护成本] → 由默认 adapter 统一负责转换，避免在主流程散落转换逻辑
- [默认路径因抽象层改坏] → 通过兼容性与语义保持测试锁定
- [抽象仍不足以支撑下一轮第二实现] → 本轮先保证边界清晰，下一轮再用第二实现验证

## Migration Plan

1. 新增 OpenSpec 工件并明确边界范围
2. 先补失败测试，锁定默认兼容性与注入语义
3. 在根包增加公开类型、接口与默认构造器
4. 将 `Agent` 改为通过接口编排
5. 更新 README 和任务状态
6. 运行 OpenSpec validate、lint、vet、race tests

## Open Questions

- 无。第二实现和 registry/factory 已明确排除在本 change 之外。
