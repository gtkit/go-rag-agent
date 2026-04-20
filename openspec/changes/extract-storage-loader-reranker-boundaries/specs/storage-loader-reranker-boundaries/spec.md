## ADDED Requirements

### Requirement: 根包公开稳定的存储、加载和 rerank 抽象
系统 SHALL 在根包公开稳定的 `VectorStore`、`DocumentLoader`、`Reranker` 接口，以及与之配套的 `Document`、`Chunk`、`ChunkRecord`、`SearchHit`、`SearchFilter` 等数据类型，使调用方可以在不依赖 `internal/*` 的前提下替换这些组件。

#### Scenario: 调用方可通过根包类型实现自定义向量存储
- **WHEN** 调用方基于根包公开的 `VectorStore` 与相关数据类型实现自定义存储
- **THEN** 该实现 MUST 可以被 `Agent` 配置接收，而不需要调用方 import `internal/storage`

#### Scenario: 调用方可通过根包类型实现自定义文档加载器
- **WHEN** 调用方基于根包公开的 `DocumentLoader` 与 `Document` 类型实现自定义加载器
- **THEN** 该实现 MUST 可以被知识导入路径接收，而不需要调用方 import `internal/rag`

#### Scenario: 调用方可通过根包类型实现自定义 reranker
- **WHEN** 调用方基于根包公开的 `Reranker` 与 `SearchHit` 类型实现自定义重排器
- **THEN** 该实现 MUST 可以被检索路径接收，而不需要调用方 import `internal/retrieval`

### Requirement: Agent 支持默认 adapter 与局部组件注入
系统 SHALL 为向量存储、文档加载和 rerank 提供默认 adapter，并允许调用方按组件粒度注入自定义实现。

#### Scenario: 默认配置继续使用默认 adapter
- **WHEN** 调用方未显式注入 `VectorStore`、`DocumentLoader` 或 `Reranker`
- **THEN** 库 MUST 分别使用默认 `Chromem` 存储、默认文件加载器和默认规则 reranker

#### Scenario: 调用方可局部替换单个组件
- **WHEN** 调用方只注入其中一个组件，例如自定义 `VectorStore`
- **THEN** 库 MUST 使用注入的该组件，并对未注入的其他组件继续使用默认 adapter

#### Scenario: 注入存储组件不改变公开问答 API
- **WHEN** 调用方通过配置注入自定义存储、加载或 rerank 组件
- **THEN** `New`、`AddKnowledge`、`Ask`、`AskWithOptions`、`AskStream`、`AskStreamWithOptions` 的公开调用方式 MUST 保持不变

### Requirement: 默认 adapter 保持当前知识导入与检索语义
系统 SHALL 在抽象公开后继续保持当前默认知识导入、检索和可选 rerank 语义不变。

#### Scenario: 默认文档加载器保持 PDF 与 OCR 语义
- **WHEN** 调用方继续使用默认文档加载器导入 Markdown、文本 PDF 或扫描版 PDF
- **THEN** 库 MUST 保持现有 front matter、sidecar metadata、PDF 直读和 OCR fallback 行为

#### Scenario: 默认向量存储保持当前过滤与删除同步语义
- **WHEN** 调用方继续使用默认向量存储进行检索和目录重导入
- **THEN** 库 MUST 保持现有 `SearchFilter` 过滤和目录删除同步语义

#### Scenario: 默认 reranker 保持当前 shortlist 重排语义
- **WHEN** 调用方启用 `EnableRerank` 且继续使用默认 reranker
- **THEN** 库 MUST 保持现有 shortlist 级规则重排行为，而不是改变为全量候选重排
