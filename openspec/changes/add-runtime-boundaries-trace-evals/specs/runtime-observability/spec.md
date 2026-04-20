## ADDED Requirements

### Requirement: 单次问答暴露结构化执行 trace
系统 SHALL 为每次同步或流式问答生成结构化执行 trace，并把它暴露给调用方消费。

#### Scenario: 同步问答成功时返回 trace
- **WHEN** 调用方成功完成一次同步问答
- **THEN** 返回的答案 MUST 附带结构化 trace，其中至少包含原始 query、rewrite 后 query、检索过滤条件、tool 调用摘要、retrieval metrics、model metrics、fallback、citation、是否流式与最终成功状态

#### Scenario: 流式问答完成时返回 trace
- **WHEN** 调用方成功完成一次流式问答
- **THEN** 库 MUST 在最终 `done` 事件中附带与该次执行对应的结构化 trace

#### Scenario: 执行失败时 recorder 仍能收到 trace
- **WHEN** 同步或流式问答在执行过程中失败，且调用方配置了 trace recorder
- **THEN** 库 MUST 仍然把包含终止错误信息的结构化 trace 发送给该 recorder

### Requirement: 日志实例可注入且可选
系统 SHALL 允许调用方注入 logger 用于输出执行摘要，并在缺省时保持 no-op 行为。

#### Scenario: 注入 logger 时输出执行摘要
- **WHEN** 调用方为 Agent 配置 logger
- **THEN** 库 MUST 在执行结束时输出与该次执行对应的摘要日志，并在发生 fallback 时输出告警日志

#### Scenario: 未注入 logger 时不影响执行
- **WHEN** 调用方未为 Agent 配置 logger
- **THEN** 库 MUST 继续正常执行问答流程，且 MUST NOT 因缺少 logger 而报错或 panic
