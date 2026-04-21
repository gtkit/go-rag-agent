## ADDED Requirements

### Requirement: PostgreSQL pgvector 作为第二个 VectorStore 实现
系统 SHALL 在保留默认 `chromem-go` embedded 实现的前提下，允许 PostgreSQL/pgvector 作为第二个 `VectorStore` 实现接入当前接口层，而不改变 `Agent` 的公开调用方式。

#### Scenario: 调用方注入 pgvector store
- **WHEN** 调用方通过 `Config.Storage.VectorStore` 注入 PostgreSQL/pgvector store
- **THEN** 库 MUST 使用该 store 完成知识导入和检索，而不要求调用方修改 `New`、`AddKnowledge`、`Ask` 或 `AskStream` 的调用方式

#### Scenario: 默认 embedded mode 保持不变
- **WHEN** 调用方未注入 PostgreSQL/pgvector store
- **THEN** 库 MUST 继续使用默认 `chromem-go` 实现，而不是隐式切换到 PostgreSQL

### Requirement: PGVectorStoreConfig 明确连接、维度与自动初始化边界
系统 SHALL 为 PostgreSQL/pgvector store 提供显式配置，至少覆盖连接来源、表名、向量维度、距离度量、自动初始化和索引策略。

#### Scenario: 连接池优先于连接串
- **WHEN** 调用方同时提供外部管理的连接池和连接串
- **THEN** store MUST 优先使用外部连接池，而不是重新创建连接

#### Scenario: 维度为必填项
- **WHEN** 调用方未提供向量维度
- **THEN** store MUST 拒绝构造，因为 `vector(n)` 需要固定维度

#### Scenario: 第一版仅正式支持 cosine
- **WHEN** 调用方为第一版配置非 `cosine` 距离度量
- **THEN** store MUST 返回明确错误，而不是静默接受未定义语义

### Requirement: pgvector 存储层保持当前 upsert、过滤与删除语义
系统 SHALL 让 PostgreSQL/pgvector 实现在当前 `VectorStore` 契约下保持与现有 store 等价的 upsert、过滤和删除语义。

#### Scenario: chunk_id upsert 语义稳定
- **WHEN** 调用方对同一 `chunk_id` 重复 upsert
- **THEN** store MUST 覆盖已有记录，而不是生成重复记录

#### Scenario: stale cleanup 继续按 parent_id 生效
- **WHEN** 同一 `parent_id` 的知识分块发生重导入，且新批次缺少旧 `chunk_id`
- **THEN** store MUST 删除这些旧 chunk，使知识库状态与当前批次保持一致

#### Scenario: SearchFilter 继续支持路径、前缀和 metadata
- **WHEN** 调用方使用 `SourcePaths`、`SourcePrefixes` 或 `Metadata` 过滤条件进行检索
- **THEN** PostgreSQL/pgvector 实现 MUST 保持与当前接口一致的过滤语义

#### Scenario: DeleteBySourcePaths 继续按来源路径删除
- **WHEN** 调用方调用 `DeleteBySourcePaths`
- **THEN** store MUST 删除所有 `source_path` 命中的记录，而不影响其他来源

### Requirement: 第一阶段默认 exact search，ANN 为可选后续能力
系统 SHALL 在 PostgreSQL/pgvector 第一版实现中默认采用 exact vector search，并将 HNSW / IVFFlat 作为可选索引能力，而不是默认启用。

#### Scenario: 默认 exact search
- **WHEN** 调用方未显式启用 ANN 索引策略
- **THEN** store MUST 使用 exact search，并保持过滤语义优先于近似索引优化

#### Scenario: HNSW 为优先的可选 ANN 策略
- **WHEN** 调用方选择启用 ANN 索引
- **THEN** 第一优先的推荐实现 MUST 是 HNSW，而不是先要求 IVFFlat

#### Scenario: ANN 与过滤交互保持显式配置
- **WHEN** 调用方在启用 ANN 的同时使用过滤条件
- **THEN** 实现 MUST 通过显式配置说明过滤场景下的召回权衡，而不能把过滤退化行为隐藏起来

### Requirement: pgvector 第二实现需要独立 integration tests
系统 SHALL 为 PostgreSQL/pgvector store 提供独立的 adapter integration tests 和最小 Agent integration test，以验证其兼容当前接口层。

#### Scenario: adapter integration tests 覆盖核心契约
- **WHEN** 仓库为 PostgreSQL/pgvector store 增加 integration tests
- **THEN** 测试 MUST 至少覆盖 upsert、search、search with filter、stale cleanup、delete by source paths、维度错误和 context cancellation

#### Scenario: 最小 Agent integration test 证明主流程兼容
- **WHEN** 调用方通过 `Config.Storage.VectorStore` 注入 PostgreSQL/pgvector store
- **THEN** 测试 MUST 能完成至少一轮 `AddKnowledge + Ask`，以证明第二实现可通过现有主流程使用
