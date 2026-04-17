## 目的

定义 Session 级短期记忆、追问解析、串行化状态变更以及取消安全执行的行为。

## 要求

### Requirement: 有界短期会话记忆
系统 SHALL 为每个 Session 维护有界短期记忆，并 SHALL NOT 在每次 prompt 中附加全部历史对话。

#### Scenario: 超出容量时裁剪旧轮次
- **WHEN** 一个 Session 追加的对话轮次超过配置的最大历史轮数
- **THEN** 库只保留最近的有界轮次

#### Scenario: 显式清空历史
- **WHEN** 调用方在有效 Session 上调用 `ClearHistory`
- **THEN** 下一次查询执行前，该 Session 历史必须已被清空

### Requirement: 追问查询解析
系统 SHALL 在最近上下文足够确定时，把指代型追问基于近期 Session 历史解析后再进行检索。

#### Scenario: 重写指代型追问
- **WHEN** 用户在一个明确的问题之后又问出类似 “how does it work?” 的指代型追问
- **THEN** 库会在 embedding 和检索前，使用近期 Session 上下文重写该检索查询

#### Scenario: 保留独立查询
- **WHEN** 用户提出一个不包含指代表达的独立查询
- **THEN** 库只做查询规范化，而不会把历史内容拼接进去

### Requirement: 同 Session 串行化状态变更
系统 SHALL 对同一个 Session 的状态变更做串行化，同时允许不同 Session 之间并发执行。

#### Scenario: 同一 Session 的并发请求
- **WHEN** 两个请求并发命中同一个 Session
- **THEN** 库会一次只执行一个该 Session 的状态变更，从而保证历史更新顺序一致

#### Scenario: 不同 Session 的并发请求
- **WHEN** 请求命中不同的 Session ID
- **THEN** 库 MAY 并发执行它们，且不共享可变 Session 状态

#### Scenario: 排队中的请求在执行前被取消
- **WHEN** 一个请求已在某个 Session 上执行，第二个同 Session 请求排队等待执行槽
- **AND** 第二个请求在拿到执行槽之前其 context 已被取消
- **THEN** 该排队请求 MUST 及时返回 context 取消错误，而不是一直等待前一个请求完成

### Requirement: 取消安全的流式执行
系统 SHALL 在流式请求被取消或回调返回错误时及时停止下游工作，并在返回前完成内部资源释放。

#### Scenario: 流式请求在执行中被取消
- **WHEN** 调用方取消一个进行中的 `AskStream` 请求的 context
- **THEN** 库会及时停止检索/模型流，并返回一个取消相关错误

#### Scenario: 回调要求停止流
- **WHEN** 调用方的 streaming callback 返回错误
- **THEN** 库 MUST 停止后续事件发出，并把该错误返回给调用方

#### Scenario: 回调 panic 不破坏 Session 最终清理
- **WHEN** telemetry callback 或 stream-emitter callback 在进行中的请求里发生 panic
- **THEN** 库 MUST 恢复该 panic，将其转换成普通错误，并仍然完成请求的 Session 最终清理路径，不能留下卡住的执行状态
