## Why

当前 hybrid retrieval 与 optional rerank 已经可用，但仍然缺少两类企业级上线前必须具备的能力：一是关键参数可调，二是可以量化比较 vector-only、hybrid 和 rerank 的性能基线。没有这两项，就很难给线上默认值和容量规划提供依据。

## What Changes

- 为 hybrid / rerank 增加可配置调优参数和稳定默认值。
- 新增 benchmark，量化 vector-only、hybrid、hybrid+r​​erank 的成本差异。
- README 更新企业级默认值建议、调参说明和 benchmark 运行方式。

## Capabilities

### New Capabilities

无

### Modified Capabilities

- `embedded-rag-library`: 修改 Agent 配置要求和检索行为，使 hybrid / rerank 的关键参数可调并有文档化默认值。

## Impact

- 受影响代码：`config.go`、`config_test.go`、`agent.go`、`internal/retrieval/*`、benchmark 文件与 `README.md`
- 受影响行为：hybrid / rerank 的候选规模、RRF 常量和 shortlist 大小不再硬编码
- 不新增外部依赖
