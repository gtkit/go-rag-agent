## Why

当前库已经支持 `Metadata` 过滤检索，但内置 `FileSource` / `DirSource` 对本地文件几乎都只提供空 metadata，这让过滤检索的价值无法真正发挥。对个人知识库最直接的提升，是让本地 Markdown front matter 和 sidecar metadata 自动进入索引，而不是要求调用方手工包装自定义 source。

## What Changes

- 为 Markdown 文件增加 YAML front matter 提取，并在导入时从正文中剥离 front matter。
- 为本地文件增加 sidecar metadata 自动发现与加载。
- 定义 metadata 合并规则，使 sidecar metadata 能覆盖 front matter 中的同名字段。
- README 更新中文用法，说明支持的 metadata 来源和限制。

## Capabilities

### New Capabilities

无

### Modified Capabilities

- `embedded-rag-library`: 修改本地知识导入要求，使 Markdown front matter 和本地 sidecar metadata 自动进入导入链路。

## Impact

- 受影响代码：`source.go`、`internal/rag/loader.go`、相关测试与 `README.md`
- 受影响行为：本地导入生成的 chunk metadata 不再总是空 map，可直接服务现有 Metadata 过滤检索
- 不新增外部服务依赖，仅使用现有 YAML 依赖
