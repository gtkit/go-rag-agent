## 目的

定义一个可嵌入 Go 应用的库优先 RAG 运行时 API。

## 要求

### Requirement: 库优先的 Agent 构造
系统 SHALL 提供一个精简的根包 API，使 Go 应用可以构造 Agent、获取 Session、导入知识源、执行同步问答、执行流式问答，并在不启动独立 HTTP 服务的前提下释放资源。

#### Scenario: 使用有效的 Phase 1 配置创建 Agent
- **WHEN** 调用方使用有效的聊天模型、embedding 模型和超时配置构造 Agent
- **THEN** 库会创建一个可用于知识导入和会话服务的 `Agent` 实例

#### Scenario: 在 Phase 1 拒绝 Phase 2 能力开关
- **WHEN** 调用方在 Phase 1 配置中启用 hybrid retrieval 或 rerank 等 Phase 2 功能
- **THEN** 库 MUST 以无效配置错误拒绝构造

### Requirement: 本地知识导入
系统 SHALL 支持从本地文件和目录中导入 `.txt`、`.md` 与 `.pdf` 知识源，对其做确定性分块、生成 embedding，并把 chunk 文本及引用元数据写入嵌入式存储。

#### Scenario: 导入支持的文件和目录来源
- **WHEN** 调用方把受支持的本地文件或目录来源传给 `AddKnowledge`
- **THEN** 库会加载其中每个 `.txt`、`.md` 与 `.pdf` 文件，进行分块、向量化，并按 source path、title、chunk ID 和 offset 元数据写入存储

#### Scenario: 拒绝不受支持的知识源类型
- **WHEN** 调用方传入的来源不是 `.txt`、`.md` 或 `.pdf`
- **THEN** 库 MUST 返回 unsupported-source 错误

#### Scenario: 从本地 PDF 中抽取文本
- **WHEN** 调用方导入一个包含可提取文本内容的本地 PDF 文件
- **THEN** 库 MUST 抽取其中的文本，并将该文本送入常规的分块与 embedding 流程

#### Scenario: 同一 store 上的并发 upsert 被串行化
- **WHEN** 两个知识导入操作并发命中同一个 store
- **THEN** 该 store MUST 一次只执行一个 upsert，使 chunk 替换和 stale cleanup 不会交错成混合状态

### Requirement: 基于检索的同步问答
系统 SHALL 基于嵌入式存储中的检索证据回答同步查询，并 SHALL 返回与所选证据对应的引用信息。

#### Scenario: 返回带引用的答案
- **WHEN** Session 在已有相关知识导入的前提下发起问题
- **THEN** 库会检索受限证据，生成答案，并同时返回答案文本和支撑该答案的引用

#### Scenario: 在证据不足时保守返回
- **WHEN** 检索到的证据为空，或不足以拼装出可用上下文
- **THEN** 库 MUST 返回证据不足或上下文拼装错误，而不是静默编造答案

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

### Requirement: 有道笔记桥接导入
系统 SHALL 提供一个知识源，通过本地安装的有道笔记 CLI 桥接命令把笔记导出到临时目录，再按常规本地文件导入流程完成导入。

#### Scenario: 通过本地桥接成功导出并导入笔记
- **WHEN** 调用方配置了有道笔记桥接知识源，且本地 `youdaonote` 命令成功把笔记导出到指定临时目录
- **THEN** 库 MUST 通过与 `DirSource` 相同的本地文件导入流程导入这些导出文件

#### Scenario: 本地桥接命令缺失
- **WHEN** 调用方使用有道笔记桥接知识源，但配置的 `youdaonote` 命令不存在
- **THEN** 库 MUST 返回一个清晰的 unsupported-source 风格错误，并指出缺失的命令名

#### Scenario: 桥接导出命令执行失败
- **WHEN** 本地 `youdaonote` 导出命令执行失败并返回错误
- **THEN** 库 MUST 把该失败作为导入错误返回，且 MUST NOT 静默继续执行空导入
