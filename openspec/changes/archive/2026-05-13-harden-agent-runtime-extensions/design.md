## Context

当前 SDK 已经具备稳定的 RAG-first 接入面：知识导入、检索增强问答、流式、citations、provider governance、prompt cache、access boundary、长期记忆 store、结构化 trace 和 eval。与 `aggo` 对比后，主要缺口不在基础 RAG，而在“企业服务如何安全地扩展 Agent runtime”：工具参数契约不够结构化，fallback 工具链不能做模型驱动的有限工具循环，记忆扩展只能落在 store 层，trace 没有开箱 recorder adapter。

本次变更必须遵守当前定位：库优先、可嵌入、默认 RAG-first。新能力只能通过显式配置启用，不能让默认问答路径变成通用 ReAct 框架，也不能引入高风险默认工具。

## Goals / Non-Goals

**Goals:**
- 为现有 `Tool` 增加向后兼容的结构化工具契约，支持 JSON Schema、结构化参数和结构化结果。
- 增加可选 tool-calling runner，支持有限迭代、工具调用预算、trace/callback 观测、context 取消和稳定错误处理。
- 增加 memory provider 边界，使调用方可以在问答前注入记忆上下文、问答后写入记忆，同时复用现有长期记忆 store。
- 增加标准 trace recorder adapters，覆盖 JSONL、组合 fan-out 和 logger bridge 等无外部 SaaS 依赖的生产基础能力。
- 增加示例、README 和回归测试，明确企业使用方式和安全边界。

**Non-Goals:**
- 不新增 CronAgent、Shell 工具、数据库执行工具或任务调度系统。
- 不引入 Milvus、Langfuse SDK、OpenTelemetry SDK 或 Eino ADK 作为核心硬依赖。
- 不替换现有 `Ask` / `AskStream` 默认 RAG-first 执行语义。
- 不改变现有 `Tool`、`LongTermMemoryStore`、`TraceRecorder` 的已有方法签名。
- 不承诺完整多 agent、sub-agent transfer 或通用 ReAct 编排框架。

## Decisions

### 1. 结构化工具采用新增接口而不是修改 `Tool`

新增接口围绕现有 `Tool` 做加法：
- `StructuredTool`：包含 `Name`、`Description`、`Schema`、`RunStructured`
- `ToolSchema` / `ToolParameterSchema`：用标准库类型表达 JSON Schema 子集
- `StructuredToolAdapter`：把结构化工具适配成现有 `Tool`
- `ToolResult`：承载文本输出、JSON 输出、是否可重试和 metadata

选择新增接口而不是扩展 `Tool.Run`，是为了不破坏现有工具实现和 fallback 工具链。结构化 schema 只覆盖企业 Agent 常用参数、required、enum、长度和数值边界；不实现完整 JSON Schema 解释器，避免把校验复杂度扩散到核心 SDK。

### 2. Tool-calling runner 显式启用，并保留默认 RAG-first runner

新增 `Config.EnableToolCalling` 和可选 `Runtime.ToolCallingRunner`。默认值保持关闭，当前 `ChatRunner` 行为不变。启用后，runner 在 prompt 中暴露工具 schema，模型通过严格 JSON envelope 请求工具调用，SDK 验证工具名、参数和预算后执行工具，再把工具结果回灌下一轮模型请求。

选择自有轻量 runner 而不是直接引入 Eino ADK，是因为当前项目的根 API、trace、retrieval、provider governance 已经成型。首版只实现单 agent、顺序工具调用、有限迭代，不支持 sub-agent 或复杂工作流。

### 3. 记忆 provider 与现有长期记忆并存

新增 `MemoryProvider`：
- `Retrieve(ctx, MemoryRetrieveRequest) (MemoryRetrieveResult, error)`
- `Memorize(ctx, MemoryMemorizeRequest) error`
- `Close() error`

`Config.Memory.Provider` 可选。问答前，SDK 调用 provider 检索系统消息、历史消息和 memory text；问答成功后，SDK 异步或同步写入 provider，具体由 provider 自己决定。现有 `LongTermMemoryStore` 继续保留；新增 adapter 把现有 long-term memory store 包成 provider，避免重复存储逻辑。

选择 provider 层而不是替换 `LongTermMemoryStore`，是为了支持外部 mem0/memu 这类服务未来独立接入，同时不破坏当前 vector-backed memory。

### 4. Trace adapters 不绑定外部 SaaS

新增 recorder adapters：
- `NewMultiTraceRecorder(recorders ...TraceRecorder)`
- `NewJSONLTraceRecorder(writer io.Writer, opts ...)`
- `NewLoggerTraceRecorder(logger Logger, opts ...)`

JSONL adapter 输出摘要视图和必要明细，但不写完整 prompt/evidence，避免敏感信息泄漏。外部 Langfuse/OTel 可以后续通过实现 `TraceRecorder` 接入，不作为本次硬依赖。

### 5. 测试按行为切分并坚持 red-green

每个新增公共行为先写 table-driven failing tests，再实现。重点覆盖：
- schema 校验 success/error/edge
- tool-calling 预算、未知工具、工具失败、context cancel、trace 记录
- memory provider retrieve/memorize 成功、失败降级、关闭
- trace recorder JSONL 敏感字段、fan-out、错误隔离
- README example 编译与公开 API GoDoc

## Risks / Trade-offs

- [模型 JSON tool-call 不稳定] → 使用严格 envelope、解析失败返回明确错误；默认不启用 tool-calling。
- [工具执行安全风险] → 核心 SDK 不提供 Shell/DB 执行工具；所有工具必须由调用方显式注册。
- [记忆写入阻塞主路径] → provider 自行决定同步/异步；SDK 必须尊重 context 并记录失败 trace，不静默 panic。
- [Trace 泄漏敏感信息] → 默认 adapters 只输出摘要和哈希/计数，不输出完整 prompt、evidence 或 API key。
- [API 面积扩大后的兼容压力] → 所有新增公开接口保持最小方法集；未来扩展通过新接口组合而不是改签名。
- [全量 race 测试耗时] → 分阶段运行 targeted tests，最终仍执行仓库要求的 lint/vet/race 全量验证。

## Migration Plan

1. 让 OpenSpec change apply-ready。
2. 先写结构化工具和 trace adapter 的失败测试，确认 API 形状。
3. 实现结构化工具接口、schema 校验和现有 `Tool` adapter。
4. 写并实现可选 tool-calling runner，接入 trace、tool limit、callbacks 和 context。
5. 写并实现 memory provider 边界和 long-term memory provider adapter。
6. 写并实现 trace recorder adapters。
7. 更新 README、examples、GoDoc 和 OpenSpec task checklist。
8. 运行 `openspec validate harden-agent-runtime-extensions --type change --strict --json --no-interactive`、`golangci-lint run ./...`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`。

## Open Questions

无。本次变更选择保守一期范围，后续 CronAgent、Milvus、Langfuse/OTel 专用适配器可以用独立 OpenSpec change 推进。
