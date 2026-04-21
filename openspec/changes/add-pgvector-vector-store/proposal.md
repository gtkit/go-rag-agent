## Why

当前库已经有稳定的 `VectorStore` 接口和默认的 `chromem-go` embedded 实现，但这只能覆盖单机、本地优先、零外部依赖的使用场景。对于需要共享知识库、持久化备份、多实例部署和数据库级运维的服务端场景，需要增加 `PostgreSQL/pgvector` 作为第二个向量存储实现，而不是直接替换默认 embedded 路径。

## What Changes

- 增加一个基于 `PostgreSQL/pgvector` 的第二个 `VectorStore` 设计方案，保持当前接口层不变
- 设计 `PGVectorStoreConfig`，明确连接、维度、距离度量、自动建表和索引策略等边界
- 设计与当前 `VectorStore` 契约匹配的建表 SQL、查询 SQL 和删除/清理语义
- 明确第一阶段与后续阶段的索引策略，优先 exact search，后续可选 HNSW / IVFFlat
- 设计 integration test 方案，覆盖 adapter 级验证与最小 Agent 级集成验证

## Capabilities

### New Capabilities
- `pgvector-vector-store`: 定义 PostgreSQL/pgvector 作为第二个 `VectorStore` 实现时必须满足的配置、存储、查询和验证边界

### Modified Capabilities

## Impact

- 受影响代码：未来将涉及根包 `VectorStore` 默认构造器、`StorageComponents` 注入路径、以及新的 Postgres adapter 实现
- 受影响依赖：新增 `pgvector-go`、`pgx/pgxpool` 类依赖的设计准备
- 受影响文档：未来 README 需要补充 embedded mode 与 server mode 的双模式说明
