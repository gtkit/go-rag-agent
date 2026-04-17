## MODIFIED Requirements

### Requirement: 基于检索的同步问答
系统 SHALL 基于嵌入式存储中的检索证据回答同步查询，并 SHALL 返回与所选证据对应的引用信息。

#### Scenario: 返回带引用的答案
- **WHEN** Session 在已有相关知识导入的前提下发起问题
- **THEN** 库会检索受限证据，生成答案，并同时返回答案文本和支撑该答案的引用

#### Scenario: 在证据不足时保守返回
- **WHEN** 检索到的证据为空，或不足以拼装出可用上下文
- **THEN** 库 MUST 返回证据不足或上下文拼装错误，而不是静默编造答案

#### Scenario: 使用检索过滤条件限制来源范围
- **WHEN** 调用方通过查询选项为单次问答指定 `source path`、`source path` 前缀或精确元数据过滤条件
- **THEN** 库 MUST 只从满足这些条件的分块中召回证据，并且最终返回的引用也 MUST 仅来自这些过滤后的结果

#### Scenario: 过滤后无证据时返回证据不足
- **WHEN** 原知识库中存在相关内容，但它们都被本次查询的检索过滤条件排除
- **THEN** 库 MUST 返回证据不足错误，而不是退回到全库检索

### Requirement: 基于回调的流式答案
系统 SHALL 提供基于回调的流式 API，发出检索/工具生命周期事件、答案分块、引用事件、错误事件，以及最终的完成事件。

#### Scenario: 成功流式输出答案分块和完成事件
- **WHEN** Session 调用 `AskStream` 且查询成功完成
- **THEN** 库会发出零个或多个答案分块事件，并最终发出 `done` 事件

#### Scenario: 在流式过程中输出引用
- **WHEN** 为流式答案选中了相关证据
- **THEN** 库 MUST 在流式过程中或之前发出与支撑 chunk 对应的引用事件

#### Scenario: emitter panic 被转换为普通错误
- **WHEN** 调用方的 streaming emitter 在库发出流式事件时发生 panic
- **THEN** 库 MUST 恢复该 panic，将其转成普通错误返回，并仍然完成 Session 的最终清理

#### Scenario: 带过滤条件的流式问答只输出过滤后的引用
- **WHEN** 调用方通过带选项的流式问答接口指定检索过滤条件
- **THEN** 库 MUST 仅输出满足过滤条件的引用和答案上下文，而不能在流式路径回退到未过滤检索
