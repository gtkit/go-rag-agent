## MODIFIED Requirements

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

#### Scenario: 对扫描版 PDF 使用 OCR fallback
- **WHEN** 调用方导入一个无法直接提取可用文本的本地 PDF，且已配置可用的 OCR bridge
- **THEN** 库 MUST 调用该 OCR bridge 提取文本，并将 OCR 文本送入常规的分块与 embedding 流程

#### Scenario: 扫描版 PDF 缺少 OCR 配置时返回清晰错误
- **WHEN** 调用方导入一个无法直接提取可用文本的本地 PDF，且当前未配置可用 OCR bridge
- **THEN** 库 MUST 返回一个清晰错误，指出该 PDF 需要 OCR 配置，而不能静默导入空内容

#### Scenario: 文本型 PDF 不强制走 OCR
- **WHEN** 调用方导入一个已经可以直接提取可用文本的本地 PDF，且同时配置了 OCR bridge
- **THEN** 库 MUST 优先使用直接文本提取结果，而不是无条件执行 OCR

#### Scenario: Markdown front matter 自动进入 metadata
- **WHEN** 调用方导入一个包含 YAML front matter 的 Markdown 文件
- **THEN** 库 MUST 把 front matter 提取为文档 metadata，并且 MUST 不把该 front matter 保留在正文分块文本中

#### Scenario: sidecar metadata 自动进入 metadata
- **WHEN** 调用方导入一个存在 sidecar metadata 文件的本地知识文件
- **THEN** 库 MUST 自动加载该 sidecar metadata，并把它合并进文档 metadata

#### Scenario: sidecar metadata 覆盖 front matter
- **WHEN** 一个 Markdown 文件同时存在 front matter metadata 和 sidecar metadata，且两者包含同名字段
- **THEN** 库 MUST 以 sidecar metadata 的值为最终导入结果

#### Scenario: 同一 store 上的并发 upsert 被串行化
- **WHEN** 两个知识导入操作并发命中同一个 store
- **THEN** 该 store MUST 一次只执行一个 upsert，使 chunk 替换和 stale cleanup 不会交错成混合状态

#### Scenario: 重导入目录时清理已删除文件
- **WHEN** 调用方成功重导入同一个 `DirSource(path)`，且该目录中有部分先前已导入文件已经从磁盘删除
- **THEN** 库 MUST 删除这些已删除文件对应的历史索引分块，使后续检索不再返回它们

#### Scenario: 空目录重导入时清理历史索引
- **WHEN** 调用方成功重导入一个先前已导入过的 `DirSource(path)`，且当前目录下已没有任何受支持文件
- **THEN** 库 MUST 删除该目录上一次成功导入留下的历史索引分块，并把本次导入视为成功完成

#### Scenario: 持久化模式跨重启保留目录删除同步
- **WHEN** 调用方在设置 `DataDir` 的模式下成功导入某个 `DirSource(path)`，随后重启 Agent，再次导入同一路径
- **THEN** 库 MUST 继续根据该目录上一次成功导入记录删除已经从目录中移除文件的历史索引分块
