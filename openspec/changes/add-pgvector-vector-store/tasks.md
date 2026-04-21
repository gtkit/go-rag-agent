## 1. 配置与依赖设计

- [x] 1.1 定义 `PGVectorStoreConfig`，明确连接来源、表名、维度、距离度量、自动初始化与索引策略字段
- [x] 1.2 选择并接入 PostgreSQL Go 客户端与 pgvector Go 库，明确连接池与类型注册方式

## 2. 存储与查询实现

- [x] 2.1 设计并实现建表、扩展初始化和基础索引创建逻辑
- [x] 2.2 实现 `Upsert`、按 `parent_id` stale cleanup、`DeleteBySourcePaths`
- [x] 2.3 实现 `Search` 和 `SearchWithFilter`，保持当前 `SearchFilter` 语义

## 3. 索引策略分阶段支持

- [x] 3.1 第一版默认 exact search，不默认启用 ANN
- [x] 3.2 增加可选 HNSW 策略及其配置参数
- [x] 3.3 预留 IVFFlat 扩展点，但不要求第一版默认启用

## 4. 验证与文档

- [x] 4.1 增加 PostgreSQL/pgvector adapter integration tests
- [x] 4.2 增加最小 Agent integration test，验证通过 `Config.Storage.VectorStore` 注入可用
- [x] 4.3 更新 README，明确 embedded mode 与 server mode 的双模式说明
