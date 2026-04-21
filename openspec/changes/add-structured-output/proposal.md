## Why

当前库已经有稳定的 RAG 检索、trace、存储和工具边界，但返回结果仍然只有自由文本 `Answer.Text`。如果调用方要把结果接到自动化流程、API 响应或 UI 组件里，还需要自己再做一次非稳健的文本解析，因此需要直接提供结构化输出能力。

## What Changes

- 为同步问答增加结构化输出 API，使调用方可以提供目标结构并得到已反序列化的 JSON 结果
- 为模型请求增加 JSON-only 输出约束，不改变现有 RAG 检索链与问答入口
- 增加结构化输出目标校验、JSON 提取和反序列化错误处理
- 保持现有 `Ask` / `AskWithOptions` / `AskStream` 行为不变，结构化输出作为新增能力提供
- 更新 README 与测试，说明结构化输出用法和约束

## Capabilities

### New Capabilities
- `structured-output`: 定义同步结构化输出 API、目标类型约束、JSON 提取与反序列化语义

### Modified Capabilities
- `embedded-rag-library`: 库优先 API 新增结构化输出入口，但不改变现有自由文本问答接口

## Impact

- 受影响代码：`session.go`、`agent.go`、`types.go`、`internal/graph/*`、新增结构化输出辅助模块与测试
- 受影响 API：新增结构化输出方法、结果类型与错误语义
- 受影响文档：`README.md`
