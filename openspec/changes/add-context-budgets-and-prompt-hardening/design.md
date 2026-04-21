## Context

当前库虽然已经有：
- `Session` 与 `MaxHistoryRounds`
- `MaxToolCalls`
- `RequestTimeout`
- execution trace

但这还不够构成真正的上下文治理：
- 历史只是按轮次裁剪，没有 token 预算
- 证据只按 rune 数限制，不按模型上下文预算治理
- 旧历史会被直接丢弃，没有摘要压缩
- 工具和检索文本会直接回灌模型，没有专门的 prompt hardening
- 缺少整轮问答的统一时长预算

这轮 change 的目标是先把“轻量但真实可用”的治理能力补齐，而不引入完整长期记忆或模型级 prompt cache。

## Goals / Non-Goals

**Goals:**
- 为 prompt 增加总预算、历史预算、证据预算和摘要预算
- 在超过历史预算时自动生成本地摘要压缩
- 为整轮问答增加统一的执行时长预算
- 为检索文本和工具回灌增加基础 prompt hardening
- 保持现有 `Ask` / `AskStream` 外部接口不变

**Non-Goals:**
- 不实现长期记忆分层
- 不实现 provider 级 prompt cache
- 不实现完整安全沙箱或高级内容安全审核
- 不引入流式结构化输出或新的模型协议

## Decisions

### 1. 预算配置直接挂在 Config 上

为了让调用方能直接配置，而不引入更多嵌套结构，新增：
- `MaxExecutionDuration`
- `MaxPromptTokens`
- `MaxHistoryTokens`
- `MaxEvidenceTokens`
- `MaxSummaryTokens`
- `EnablePromptHardening`

默认值设计为“对现有行为影响最小但真实生效”：
- `MaxPromptTokens = 4096`
- `MaxHistoryTokens = 1024`
- `MaxEvidenceTokens = 2048`
- `MaxSummaryTokens = 256`
- `EnablePromptHardening = true`
- `MaxExecutionDuration = 0` 表示默认不额外收紧总时长

### 2. Prompt 治理放在 graph prompt 构造层

历史压缩、证据 token 截断和不可信文本硬化都放在 `internal/graph` 的 prompt 构造层，而不是散落在 `Agent` 多处路径里。这样同步和流式问答、fallback 工具回灌都会自动走同一套逻辑。

### 3. 历史压缩使用本地摘要，而不是额外模型调用

第一版的“摘要压缩”不新增一次模型总结调用，而是对被裁掉的旧历史做 deterministic summary：
- 保留最近轮次
- 对旧轮次生成简短文本摘要
- 摘要本身也受 `MaxSummaryTokens` 限制

这样可以在不增加额外成本和失败面的情况下先建立治理能力。

### 4. Prompt hardening 以“包裹 + 过滤明显模式”为主

对检索文本和工具输出：
- 包裹为“不可信上下文”
- 对命中明显 prompt injection 模式的行替换成固定标记

这不是完整安全防线，但能显式降低“把工具返回值当系统指令”的风险。

### 5. 统一执行时长预算通过 context cause 实现

`MaxExecutionDuration` 使用 `context.WithTimeoutCause` 包住整轮执行。这样：
- 检索、工具和模型调用都受统一预算约束
- 超时后调用方可通过 `ErrExecutionBudgetExceeded` 区分这是“整轮预算”而非普通外部请求超时

## Risks / Trade-offs

- [本地摘要压缩不如模型摘要准确] → 第一版优先低复杂度和稳定性，后续如有需要再接模型级摘要
- [prompt hardening 可能误伤正常文本] → 只过滤最明显模式，并保留可关闭开关
- [新增预算可能影响旧测试或默认行为] → 默认值尽量宽松，并通过兼容测试锁住现有路径

## Migration Plan

1. 补 proposal/design/spec/tasks
2. 先写失败测试，锁 prompt 压缩、hardening 和执行预算
3. 在 `Config` 增加预算与硬化配置
4. 在 `internal/graph` 实现 prompt 治理
5. 在 `Agent` 接入统一执行时长预算
6. 更新 README、任务状态并跑完整验证

## Open Questions

- 无。长期记忆、provider prompt cache 和高级安全治理已明确排除在本 change 外。
