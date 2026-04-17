## Context

仓库已经具备：
- 本地 `.txt` / `.md` / `.pdf` 导入
- 过滤检索（`Metadata` 精确匹配）
- OCR bridge 与目录同步

但 `FileSource` / `DirSource` 当前生成的 `KnowledgeFile.Metadata` 基本恒为空，只有调用方手工实现自定义 source 才能填 metadata。这导致过滤检索虽然可用，却缺少内置数据来源支撑。

对于个人知识库，最常见且性价比最高的内置 metadata 来源是：
- Markdown YAML front matter
- 与正文文件并列的 sidecar metadata 文件

## Goals / Non-Goals

**Goals：**

- 为 `.md` 支持 YAML front matter 提取
- 提取后把 front matter 从正文中移除，避免污染分块文本
- 为所有支持的知识文件支持 sidecar metadata 文件
- 定义清晰的 metadata 合并优先级

**Non-Goals：**

- 不实现复杂目录级规则继承
- 不实现通用模板语言或 metadata DSL
- 不实现模糊/范围 metadata 过滤
- 不支持跨多个文件聚合 metadata

## Decisions

### 1. front matter 在 loader 层解析，而不是 source 层

选择：Markdown front matter 提取放在 `internal/rag/loader.go`。

原因：
- loader 已经负责“把文件内容变成 Document”
- front matter 不只是 metadata 提取，还需要从正文中剥离
- 这样 `KnowledgeSource` 仍只负责文件发现，职责边界更清晰

### 2. sidecar metadata 在 source 层解析

选择：`FileSource` / `DirSource` 自动查找同文件 sidecar metadata，并填入 `KnowledgeFile.Metadata`。

原因：
- sidecar metadata 与文件发现强绑定
- 不需要读取正文内容即可完成
- 可以对 `.txt` / `.md` / `.pdf` 统一生效

### 3. sidecar metadata 覆盖 front matter 同名字段

选择：合并顺序为：
1. front matter metadata
2. sidecar metadata

后写覆盖前写。

原因：
- sidecar 是文件外部的显式补充，更适合覆盖正文内声明
- 对 PDF / TXT 这种没有 front matter 的文件也保持统一语义

### 4. sidecar 文件命名采用 `<basename>.meta.{json|yaml|yml}`

选择：例如：
- `note.md` 对应 `note.meta.json`
- `paper.pdf` 对应 `paper.meta.yaml`

原因：
- 命名简洁，避免把正文扩展名重复进 sidecar 文件名
- 对所有支持源类型统一

### 5. metadata 值统一字符串化

选择：支持顶层对象；标量值直接转字符串，复合值序列化为稳定 JSON 字符串。

原因：
- 与当前 `map[string]string` 契约兼容
- 不扩大现有过滤接口

## Risks / Trade-offs

- [front matter 语法错误导致导入失败] → 明确返回解析错误，避免静默错误 metadata
- [复合值字符串化后不够“自然”] → 当前过滤本来就是精确匹配，先保证可存储与可诊断
- [sidecar 文件被误当正文导入] → 目录遍历时显式跳过 `.meta.json/.yaml/.yml`

## Migration Plan

1. 新增 source 侧 sidecar metadata 解析测试
2. 新增 loader 侧 front matter 提取与正文清洗测试
3. 实现 sidecar 发现、front matter 提取和 metadata 合并
4. 更新 README

## Open Questions

无
