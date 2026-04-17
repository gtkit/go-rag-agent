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

#### Scenario: 本地证据不足时允许进入联网搜索路径
- **WHEN** 本地检索返回证据不足，且调用方已启用联网搜索配置
- **THEN** 库 MUST 允许问答流程进入带工具的联网搜索路径，而不是直接返回证据不足

#### Scenario: 新执行后端下本地证据仍然优先
- **WHEN** 运行时使用非 Eino 的替代执行后端，且本地检索已返回足够证据
- **THEN** 库 MUST 优先基于本地证据生成答案，而不能无条件触发联网搜索

#### Scenario: hybrid retrieval 可以提升 lexical 命中的证据排序
- **WHEN** 调用方启用 `EnableHybridSearch`，且查询包含强 lexical 信号而纯向量排序无法把最佳证据排到前列
- **THEN** 库 MUST 通过 vector + lexical 融合，把更合适的候选提升到最终证据集合中

#### Scenario: optional rerank 仅作用于 shortlist
- **WHEN** 调用方同时启用 `EnableHybridSearch` 和 `EnableRerank`
- **THEN** 库 MUST 只对 hybrid 召回后的 shortlist 执行重排，而不是对全量候选做 rerank

#### Scenario: 调整参数不会改变检索边界语义
- **WHEN** 调用方修改 hybrid candidate multiplier、RRF 常量或 rerank shortlist multiplier
- **THEN** 库 MAY 改变排序和成本，但 MUST 继续遵守已有的 source path / metadata 过滤边界

#### Scenario: hybrid 内部失败时退回 vector-only
- **WHEN** 调用方启用 `EnableHybridSearch`，但 hybrid 内部阶段失败
- **THEN** 库 MUST 退回到 vector-only 检索，而不是直接中断整个问答

#### Scenario: rerank 内部失败时退回 hybrid
- **WHEN** 调用方启用 `EnableRerank`，但 rerank 阶段失败
- **THEN** 库 MUST 退回到未 rerank 的 hybrid 结果，而不是直接中断整个问答

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

#### Scenario: hybrid retrieval 与 rerank 在流式路径同样生效
- **WHEN** 调用方在流式问答中启用 `EnableHybridSearch` 或 `EnableRerank`
- **THEN** 库 MUST 在流式路径使用与同步问答一致的检索和排序策略

#### Scenario: 本地证据不足时流式路径也允许联网搜索
- **WHEN** 流式问答中的本地检索返回证据不足，且调用方已启用联网搜索配置
- **THEN** 库 MUST 进入带工具的联网搜索路径，而不是直接结束流式问答

#### Scenario: 新执行后端下流式路径保持相同搜索语义
- **WHEN** 运行时使用非 Eino 的替代执行后端
- **THEN** 库 MUST 在流式路径中保持与同步路径一致的本地检索优先和证据不足后联网搜索语义
