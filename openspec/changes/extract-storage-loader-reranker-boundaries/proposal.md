## Why

当前库虽然已经有可注入的聊天模型与 embedding 运行时边界，但向量存储、文档加载和 rerank 仍主要停留在 `internal/*` 实现细节中。要让这个库继续沿着“稳定 RAG 内核”方向演进，而不是继续绑死当前实现，必须先把这三块公开成稳定抽象。

## What Changes

- 在根包公开稳定的 `VectorStore`、`DocumentLoader`、`Reranker` 接口及其配套数据类型
- 为当前 `ChromemStore`、文件加载器和规则 rerank 提供默认 adapter 构造器
- 为 `Config` 增加独立的存储/加载/rerank 组件注入结构，使 `Agent` 可以通过接口而不是内部实现编排
- 保持默认行为不变，不在本 change 中接入第二套实现
- 更新 README 与测试，说明新的边界和默认 adapter 语义

## Capabilities

### New Capabilities
- `storage-loader-reranker-boundaries`: 定义根包公开的向量存储、文档加载和 rerank 抽象及默认 adapter 行为

### Modified Capabilities

## Impact

- 受影响代码：`config.go`、`agent.go`、根包类型定义、`internal/storage/*`、`internal/rag/*`、`internal/retrieval/*`、相关测试
- 受影响 API：新增根包公开接口、数据类型、默认构造器与注入配置
- 受影响文档：`README.md`
