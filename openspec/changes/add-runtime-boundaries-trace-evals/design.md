## Context

当前根包已经有 `Logger`、`Callback`、`RetrievalMetrics` 和 `ModelMetrics` 等对外观测类型，但真正的聊天模型与 embedding 运行时抽象仍停留在 `internal/llm`，`Agent.New` 也会无条件使用默认 OpenAI-compatible 配置去构造它们。结果是：

- 调用方如果想接入自己的 provider 适配层，必须改内部实现，无法只通过根包 API 完成。
- 单次执行的观测信息分散在多个 callback 和流式事件里，调用方需要自行拼接才能看出一轮问答到底做了什么。
- 仓库测试对核心行为有覆盖，但缺少一组明确围绕 “runtime 注入 + trace 形状” 的稳定回归集。

这次变更保持当前库优先形态和默认 LangChainGo 路径不变，但把 runtime、trace 和 regression 这三层基础设施补齐。

## Goals / Non-Goals

**Goals:**
- 在根包公开稳定的 `ChatModel` / `Embedder` / 消息类型边界，允许外部代码不依赖 `internal/*` 即可接入自定义 provider
- 保持默认 OpenAI-compatible + LangChainGo 实现可用，并作为未注入 runtime 时的默认路径
- 为 `Ask` / `AskStream` 补齐结构化执行 trace，并允许通过 recorder 与 logger 消费
- 在仓库测试中建立确定性的回归评测集，锁定 runtime 注入、同步 trace 与流式 trace

**Non-Goals:**
- 不把 `go-llm-provider` 设为本次变更的硬依赖
- 不重写 retrieval、memory、storage 或 web search 的既有业务语义
- 不把回归评测集设计成依赖真实外部模型输出的在线 benchmark
- 不新增 HTTP 服务或独立评测命令行

## Decisions

### 1. 在根包公开 runtime 边界，但保留内部实现包

根包会公开 `ChatModel`、`Embedder`、消息角色与默认 OpenAI-compatible 构造器；其底层继续复用 `internal/llm` 的现有实现。调用方只需要 import 根包即可实现自己的 provider 适配器，不必碰 `internal/*`。

选择这个方案而不是把 `internal/llm` 整体搬到根包，是为了保持内部执行模块结构稳定，同时最小化 API 扩散面。

### 2. 用 `RuntimeComponents` 做注入，而不是引入新的全局工厂层

`Config` 会新增一个运行时组件字段，用来承载可选的 `ChatModel` / `Embedder` 注入。某个组件未注入时，仍按现有配置构造默认实现；只有缺失组件对应的默认配置才会触发校验失败。

这样可以同时支持三种路径：全部默认、全部注入、半注入半默认。相比额外引入 provider registry / factory registry，这个方案更小、更稳定，也更适合作为后续接入 `go-llm-provider` 的桥接点。

### 3. 单次执行 trace 作为结果的一部分，同时支持可选 recorder

同步问答会把 trace 附加到 `Answer`；流式问答会在 `done` 或错误事件中附带 trace。除此之外，`Config` 会新增可选 `TraceRecorder`，保证即使同步路径最终返回错误，也能把完整 trace 发给调用方。

这样做的原因是：
- 结果携带 trace 适合直接调试和单测断言
- recorder 适合落库、聚合或在线诊断
- 两者都不要求调用方实现完整 callback 接口

### 4. logger 只做摘要输出，并保持可注入 / 可缺省

已有 `Logger` 接口会真正接入执行路径，但只输出摘要级结构化日志，例如 session、query rewrite、命中数、fallback、耗时和终态，不额外引入新的日志依赖。未提供 logger 时，使用 no-op 路径，不能改变行为。

相比让 trace 逻辑直接依赖某个具体日志包，这个方案满足注入需求，也避免把库绑死到单一 logger 实现。

### 5. 回归评测集使用固定 stub，而不是在线调用真实 provider

新增的 regression suite 会使用固定 embedder/chat model stub、内存知识数据和表驱动 case，断言运行时组件注入、trace 字段和流式 trace 终态。这样 `go test` 就能稳定执行，不受网络、模型版本或温度参数波动影响。

## Risks / Trade-offs

- [公开 runtime 抽象后需要长期兼容] → 公开接口保持最小集，只暴露当前真正需要稳定的聊天/embedding 契约
- [trace 字段增加结果对象大小] → trace 只记录摘要信息，不复制全文 evidence，也不记录未选中的 chunk 文本
- [logger / recorder 来自调用方，可能 panic] → 继续通过防御性回调包装恢复 panic，避免影响 Agent 清理
- [graph 层需要额外暴露 tool 调用观测钩子] → 仅增加最小内部 observer，用于 `search_web` trace，不向外扩展新的复杂协议

## Migration Plan

1. 新增 OpenSpec artifacts，并让 change 进入 apply-ready
2. 先写失败测试，锁定 runtime 注入、同步 trace、流式 trace 与 logger/recorder 语义
3. 实现根包 runtime 边界与默认构造器接线
4. 实现 trace builder、recorder/logger 接线与 graph 内部 tool 观测
5. 增加 regression suite，更新 README 与任务状态
6. 运行 `openspec validate`、`golangci-lint`、`go vet`、`go test -race`

## Open Questions

- 无。本次 change 明确不把 `go-llm-provider` 作为强依赖，而是先稳定注入边界。
