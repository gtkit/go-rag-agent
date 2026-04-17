## Why

当前检索只支持“全知识库相似度召回”，当知识库规模变大或内容来自多个目录/标签来源时，调用方无法把问题限制在某个来源范围内，容易引入无关证据。既然本地导入、引用返回和目录同步已经稳定，下一步最有价值的能力就是让调用方按来源和元数据收窄检索范围。

## What Changes

- 新增公开查询选项结构，允许调用方为单次问答指定检索过滤条件。
- 新增带选项的 Session 查询入口，保留现有 `Ask` / `AskStream` 兼容行为不变。
- 扩展向量检索契约，支持按 `source path`、`source path` 前缀和精确元数据匹配过滤结果。
- README 更新中文示例，说明何时适合使用过滤检索。

## Capabilities

### New Capabilities

无

### Modified Capabilities

- `embedded-rag-library`: 修改检索问答要求，使调用方可以对单次检索指定 `source path`、目录前缀和元数据过滤条件。

## Impact

- 受影响代码：`types.go`、`session.go`、`agent.go`、`internal/storage/*`、相关测试与 `README.md`
- 受影响 API：新增查询选项类型与带选项的 Session 查询方法
- 无新增第三方依赖
