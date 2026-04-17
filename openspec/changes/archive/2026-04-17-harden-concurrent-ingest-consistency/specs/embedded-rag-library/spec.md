## MODIFIED Requirements

### Requirement: 本地知识导入
系统 SHALL 支持从本地文件和目录中导入 `.txt`、`.md` 与 `.pdf` 知识源，对其做确定性分块、生成 embedding，并把 chunk 文本及引用元数据写入嵌入式存储。

#### Scenario: 导入支持的文件和目录来源
- **WHEN** 调用方把受支持的本地文件或目录来源传给 `AddKnowledge`
- **THEN** 库会加载其中每个 `.txt`、`.md` 与 `.pdf` 文件，进行分块、向量化，并按 source path、title、chunk ID 和 offset 元数据写入存储

#### Scenario: 拒绝不受支持的知识源类型
- **WHEN** 调用方传入的来源不是 `.txt`、`.md` 或 `.pdf`
- **THEN** 库 MUST 返回 unsupported-source 错误

#### Scenario: 同一 store 上的并发 upsert 被串行化
- **WHEN** 两个知识导入操作并发命中同一个 store
- **THEN** 该 store MUST 一次只执行一个 upsert，使 chunk 替换和 stale cleanup 不会交错成混合状态
