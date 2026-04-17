## Why

当前 `DirSource(path)` 重复导入时只能新增或覆盖仍然存在的文件，已经从目录里删除的文件仍会保留在索引里，继续参与检索和引用返回。个人知识库一旦开始持续维护，这会直接产生陈旧答案和错误引用，因此需要把目录导入补成“以当前目录内容为准”的同步语义。

## What Changes

- 修改 `DirSource(path)` 导入语义：成功重导入同一目录根路径时，自动删除该目录下已从磁盘移除文件对应的历史索引分块。
- 为目录导入增加删除感知状态记录；持久化模式下把该状态保存在 `DataDir` 下，内存模式下保存在进程内。
- 扩展向量存储契约，支持按 `source path` 批量删除已索引分块。
- 保持 `FileSource(...)` 和有道笔记桥接导入的现有行为不变，不把它们提升成目录级删除同步来源。

## Capabilities

### New Capabilities

无

### Modified Capabilities

- `embedded-rag-library`: 修改本地目录知识导入要求，使 `DirSource(path)` 在成功重导入同一路径时删除已经从目录中移除文件的历史索引结果。

## Impact

- 受影响代码：`session.go`、`source.go`、`agent.go`、`internal/storage/*`、相关测试与 `README.md`
- 受影响行为：`AddKnowledge(ctx, DirSource(path))` 从“只增不删”改为“目录内容对齐”
- 受影响持久化：在 `DataDir` 模式下新增目录导入状态文件，用于跨重启保留删除同步语义
