## Context

当前库已经完成 `VectorStore` 接口层抽象，并保留 `chromem-go` 作为默认 embedded 实现。这条路径适合本地优先、单进程、零外部依赖的场景，但不适合需要共享知识库、数据库备份、SQL 过滤和多实例部署的服务端模式。

因此下一步不应直接把 `chromem-go` 替换成 PostgreSQL，而是继续保留 embedded mode，同时增加 `PostgreSQL/pgvector` 作为第二实现。这样做能同时保留本地模式和服务端模式，而不会改变当前库的产品定位。

## Goals / Non-Goals

**Goals:**
- 设计一个与现有 `VectorStore` 契约兼容的 `pgvector` 第二实现
- 明确 `PGVectorStoreConfig` 的边界和默认行为
- 明确建表 SQL、查询 SQL、索引策略和 integration test 方案
- 保持当前接口层与默认 `chromem-go` 路径不变

**Non-Goals:**
- 不直接替换默认 `chromem-go` 实现
- 不在本草稿中实现代码
- 不扩展 `VectorStore` 现有接口
- 不在第一版就设计 registry 或字符串型后端选择

## Decisions

### 1. `PGVectorStoreConfig`

第一版配置建议收敛为：

- `Pool *pgxpool.Pool`
- `ConnString string`
- `SchemaName string`
- `TableName string`
- `Dimensions int`
- `DistanceMetric string`
- `AutoCreateExtension bool`
- `AutoCreateSchema bool`
- `AutoCreateTable bool`
- `AutoCreateIndexes bool`
- `IndexStrategy string`
- `HNSWM int`
- `HNSWEfConstruction int`
- `HNSWEfSearch int`
- `IVFFlatLists int`
- `IVFFlatProbes int`

设计判断：
- `Pool` 与 `ConnString` 二选一，优先允许调用方传入外部已管理的 `pgxpool.Pool`
- `SchemaName` 默认 `public`
- `TableName` 必填，不复用单表多 collection 设计
- `Dimensions` 必填，因为 `vector(n)` 需要固定维度
- 第一版只正式支持 `DistanceMetric=cosine`

选择 `cosine` 的原因：
- 当前库的 `SearchHit.Score` 和 `threshold` 语义更像相似度而不是距离
- 在 SQL 层最容易映射为 `score = 1 - distance`
- 避免同时为 cosine / l2 / inner product 定义多套分数语义

替代方案：
- 同时开放多种 metric。否决原因：会扩大第一版契约复杂度。

### 2. 建表 SQL

第一版表结构采用显式列，而不是把所有业务字段都塞进 `jsonb`：

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS ragagent;

CREATE TABLE IF NOT EXISTS ragagent.knowledge_chunks (
    chunk_id text PRIMARY KEY,
    parent_id text NOT NULL,
    source_path text NOT NULL,
    title text NOT NULL,
    content text NOT NULL,
    start_rune integer NOT NULL,
    end_rune integer NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    embedding vector(1536) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS knowledge_chunks_parent_id_idx
    ON ragagent.knowledge_chunks (parent_id);

CREATE INDEX IF NOT EXISTS knowledge_chunks_source_path_idx
    ON ragagent.knowledge_chunks (source_path);

CREATE INDEX IF NOT EXISTS knowledge_chunks_source_path_prefix_idx
    ON ragagent.knowledge_chunks (source_path text_pattern_ops);

CREATE INDEX IF NOT EXISTS knowledge_chunks_metadata_gin_idx
    ON ragagent.knowledge_chunks
    USING gin (metadata jsonb_path_ops);
```

关键设计判断：
- `chunk_id` 作为主键，直接匹配现有 upsert 语义
- `parent_id` 保留为显式列，因为当前 stale cleanup 需要按 parent 删除旧 chunk
- `metadata` 用 `jsonb`
- `source_path` 和 `source_path text_pattern_ops` 用于 `SourcePaths` / `SourcePrefixes`

### 3. 查询 SQL

第一版 adapter 保持与现有 `VectorStore` 契约对齐，主要 SQL 包括：

`Upsert`：

```sql
INSERT INTO ragagent.knowledge_chunks (
    chunk_id,
    parent_id,
    source_path,
    title,
    content,
    start_rune,
    end_rune,
    metadata,
    embedding,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, now()
)
ON CONFLICT (chunk_id) DO UPDATE SET
    parent_id = EXCLUDED.parent_id,
    source_path = EXCLUDED.source_path,
    title = EXCLUDED.title,
    content = EXCLUDED.content,
    start_rune = EXCLUDED.start_rune,
    end_rune = EXCLUDED.end_rune,
    metadata = EXCLUDED.metadata,
    embedding = EXCLUDED.embedding,
    updated_at = now();
```

`Delete stale chunks by parent_id`：

```sql
DELETE FROM ragagent.knowledge_chunks
WHERE parent_id = $1
  AND NOT (chunk_id = ANY($2::text[]));
```

`Search`：

```sql
SELECT
    chunk_id,
    parent_id,
    source_path,
    title,
    content,
    start_rune,
    end_rune,
    metadata,
    1 - (embedding <=> $1) AS score
FROM ragagent.knowledge_chunks
ORDER BY embedding <=> $1
LIMIT $2;
```

`SearchWithFilter`：

```sql
SELECT
    chunk_id,
    parent_id,
    source_path,
    title,
    content,
    start_rune,
    end_rune,
    metadata,
    1 - (embedding <=> $1) AS score
FROM ragagent.knowledge_chunks
WHERE 1 = 1
  AND ($2::text[] IS NULL OR source_path = ANY($2))
  AND ($3::text[] IS NULL OR source_path LIKE ANY($3))
  AND ($4::jsonb IS NULL OR metadata @> $4)
ORDER BY embedding <=> $1
LIMIT $5;
```

`DeleteBySourcePaths`：

```sql
DELETE FROM ragagent.knowledge_chunks
WHERE source_path = ANY($1::text[]);
```

设计判断：
- `SourcePaths` 用 `= ANY(...)`
- `SourcePrefixes` 由 Go 层先转换成 `prefix%`，SQL 用 `LIKE ANY(...)`
- `Metadata` 用 `metadata @> ...`
- 第一版继续以 exact filter 语义为准

### 4. 索引策略

阶段化建议如下：

**Phase 1**
- 默认 `IndexStrategy=none`
- 只做 exact search
- 目标是先把 adapter 语义做稳，而不是先做近似索引优化

**Phase 2**
- 增加可选 `HNSW`
- 索引 SQL：

```sql
CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_hnsw_cosine_idx
    ON ragagent.knowledge_chunks
    USING hnsw (embedding vector_cosine_ops);
```

- 推荐默认参数：
  - `m = 16`
  - `ef_construction = 64`
  - `ef_search = 100`

**Phase 3**
- 如有确切 bulk load / 超大规模需求，再增加 `IVFFlat`
- 索引 SQL：

```sql
CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_ivfflat_cosine_idx
    ON ragagent.knowledge_chunks
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
```

索引策略判断：
- 第一版不默认开启 ANN
- 后续优先上 `HNSW`，不优先 `IVFFlat`
- 原因是 `pgvector` 官方文档对 HNSW 的 speed-recall 折中更积极，且更适合 RAG 查询

同时需要注意过滤问题：
- `pgvector` 的 approximate index 在过滤场景下可能因为后过滤而减少返回结果
- 如果后续启用 HNSW，建议配合 `hnsw.iterative_scan = strict_order`

### 5. integration test 方案

集成测试分两层：

**Adapter integration tests**

1. `TestPGVectorStoreUpsertAndSearch`
- 固定向量写入
- exact search
- 校验返回顺序、分数字段和 metadata 映射

2. `TestPGVectorStoreSearchWithFilter`
- 校验 `SourcePaths`
- 校验 `SourcePrefixes`
- 校验 `Metadata`
- 校验组合过滤

3. `TestPGVectorStoreUpsertDeletesStaleChunksByParent`
- 同一 `parent_id` 先写入多条 chunk
- 第二次 upsert 缩短集合
- 校验旧 chunk 被清理

4. `TestPGVectorStoreDeleteBySourcePaths`
- 写入多个 `source_path`
- 删除其中一组
- 校验仅目标路径被删除

5. `TestPGVectorStoreDimensionMismatch`
- 写入错误维度向量
- 校验返回明确错误

6. `TestPGVectorStoreContextCancellation`
- 取消 context 后执行 search/upsert
- 校验及时返回

7. `TestPGVectorStoreHNSWSmoke`
- 仅在启用 HNSW 配置时执行
- 校验索引创建和查询可用
- 不在这组测试里做 recall 基准断言

**Agent integration test**

8. `TestAgentWithPGVectorStore`
- 通过 `Config.Storage.VectorStore` 注入 `PGVectorStore`
- 其他组件继续默认
- 执行一轮 `AddKnowledge + Ask`
- 校验第二实现能走完整主流程

测试环境建议：
- 使用独立 PostgreSQL + `pgvector` 测试实例
- 每个测试使用独立表名
- 初始化时执行 `CREATE EXTENSION IF NOT EXISTS vector`
- 使用 `pgvector-go/pgx` 注册类型

## Risks / Trade-offs

- [直接替换默认存储会改变产品定位] → 明确保留 `chromem-go` 为默认 embedded mode，仅增加第二实现
- [ANN 索引与过滤交互复杂] → 第一版默认 exact search；ANN 延后到后续阶段
- [维度固定带来 schema 约束] → `Dimensions` 设为显式必填配置
- [自动建表与外部迁移责任边界不清] → 配置中显式控制 `AutoCreate*` 行为

## Migration Plan

1. 先完成 `pgvector` 第二实现的 proposal/design/spec
2. 第一版只实现 exact search 的 `VectorStore` adapter
3. 完成 adapter integration tests 和最小 Agent integration test
4. README 补充 embedded mode 与 server mode 双模式说明
5. 后续按阶段增加 HNSW / IVFFlat

## Open Questions

- 第一版是否允许 adapter 自动执行 `CREATE EXTENSION vector`
- 第一版是每个 store 对应一张表，还是同表多 collection
- HNSW 默认是否启用，还是保持 opt-in
