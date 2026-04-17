## MODIFIED Requirements

### Requirement: 库优先的 Agent 构造
系统 SHALL 提供一个精简的根包 API，使 Go 应用可以构造 Agent、获取 Session、导入知识源、执行同步问答、执行流式问答，并在不启动独立 HTTP 服务的前提下释放资源。

#### Scenario: 使用有效的 Phase 1 配置创建 Agent
- **WHEN** 调用方使用有效的聊天模型、embedding 模型和超时配置构造 Agent
- **THEN** 库会创建一个可用于知识导入和会话服务的 `Agent` 实例

#### Scenario: 启用 hybrid retrieval
- **WHEN** 调用方在配置中启用 `EnableHybridSearch`
- **THEN** 库 MUST 允许构造成功，并在后续检索中同时使用向量召回和 lexical recall

#### Scenario: 启用 rerank 但未启用 hybrid retrieval
- **WHEN** 调用方启用 `EnableRerank` 但未启用 `EnableHybridSearch`
- **THEN** 库 MUST 以无效配置错误拒绝构造

#### Scenario: hybrid / rerank 调优参数使用稳定默认值
- **WHEN** 调用方未显式配置 hybrid candidate multiplier、RRF 常量和 rerank shortlist multiplier
- **THEN** 库 MUST 使用文档化且稳定的默认值运行，而不是依赖散落在代码里的隐式常量

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

#### Scenario: hybrid retrieval 可以提升 lexical 命中的证据排序
- **WHEN** 调用方启用 `EnableHybridSearch`，且查询包含强 lexical 信号而纯向量排序无法把最佳证据排到前列
- **THEN** 库 MUST 通过 vector + lexical 融合，把更合适的候选提升到最终证据集合中

#### Scenario: optional rerank 仅作用于 shortlist
- **WHEN** 调用方同时启用 `EnableHybridSearch` 和 `EnableRerank`
- **THEN** 库 MUST 只对 hybrid 召回后的 shortlist 执行重排，而不是对全量候选做 rerank

#### Scenario: 调整参数不会改变检索边界语义
- **WHEN** 调用方修改 hybrid candidate multiplier、RRF 常量或 rerank shortlist multiplier
- **THEN** 库 MAY 改变排序和成本，但 MUST 继续遵守已有的 source path / metadata 过滤边界
