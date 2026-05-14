## Why

与 `aggo` 对比后，当前 SDK 的 RAG、引用、pgvector、prompt cache、provider governance、trace/eval 等核心能力已经较完整，但通用 Agent 扩展面仍偏弱：工具缺少结构化参数契约，执行器不能按模型意图进行有限工具循环，长期记忆只暴露 store 级接口，trace 只能通过调用方自建 recorder 消费。现在补齐这些能力，可以让 SDK 更适合企业服务嵌入场景，同时继续保持“库优先、RAG 优先、非控制台”的定位。

## What Changes

- 新增结构化工具契约与适配器，让工具可以声明 JSON Schema、接收结构化参数并返回结构化结果，同时保持现有 `Tool` 接口兼容。
- 新增可选的 tool-calling runner，使调用方显式启用后可以执行有限、可观测、受 `MaxToolCalls` 和 `MaxIterations` 约束的工具调用循环。
- 新增记忆 provider 边界，把“检索注入上下文”和“问答后写入记忆”抽象成可插拔接口，并提供基于现有长期记忆 store 的默认适配器。
- 新增 trace recorder 适配能力，提供面向日志/JSONL/组合 recorder 的标准实现，复用现有 `ExecutionTrace`，不强绑定外部 SaaS。
- 增加示例和 README 文档，说明结构化工具、tool-calling、记忆 provider 和 trace adapter 的使用方式及安全边界。
- 不引入 CronAgent、Shell/数据库执行工具或 Milvus 作为本次核心默认能力；这些能力安全边界和依赖成本更高，适合后续独立变更。

## Capabilities

### New Capabilities
- `agent-runtime-extensions`: 定义结构化工具、可选 tool-calling runner、记忆 provider 边界和相关生产安全约束。

### Modified Capabilities
- `embedded-rag-library`: Agent 构造和问答流程允许调用方显式启用 tool-calling runner 与 memory provider，同时保持现有 RAG-first 默认路径。
- `runtime-observability`: 运行时观测新增标准 trace recorder 适配器和 tool-calling 相关 trace 字段消费语义。
- `example-tests`: 示例测试覆盖结构化工具、tool-calling、memory provider 和 trace adapter 的嵌入式用法。

## Impact

- 受影响代码：`config.go`、`agent.go`、`session.go`、`tool_registry.go`、`runtime.go`、`trace.go`、`long_term_memory.go`、`internal/graph/*`、新增工具/记忆/trace adapter 文件、相关测试和示例。
- 受影响 API：新增可选接口、配置字段和构造器；不移除现有公开 API，不改变 `Tool`、`Reranker`、`VectorStore`、`LongTermMemoryStore` 的已有方法签名。
- 受影响行为：默认构造和默认问答仍走当前 RAG-first 路径；只有显式启用新 runner/provider 时才改变运行时行为。
- 受影响文档：`README.md`、example tests、OpenSpec specs/tasks。
