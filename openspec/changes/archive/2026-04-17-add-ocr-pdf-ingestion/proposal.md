## Why

当前库只支持“可直接抽取文本”的 PDF，扫描件 PDF 仍然无法导入，这会让大量真实个人知识库资料停留在库外。既然本地文本 PDF、目录同步和过滤检索已经稳定，下一步最直接的可用性提升就是补齐扫描版 PDF 的 OCR 导入路径。

## What Changes

- 为本地 `.pdf` 导入增加 OCR fallback 路径，用于扫描版或图片型 PDF。
- 新增可配置的 OCR bridge 配置，让调用方通过本地 OCR 命令完成识别，而不是把某个特定 OCR 服务硬编码进库。
- 保持现有文本型 PDF 快路径不变；只有在直接文本提取为空或不足时才进入 OCR。
- README 补充 OCR 使用方式、依赖前提和限制说明。

## Capabilities

### New Capabilities

无

### Modified Capabilities

- `embedded-rag-library`: 修改本地知识导入要求，使本地 PDF 在无法直接提取文本时可通过配置的 OCR bridge 导入扫描版内容。

## Impact

- 受影响代码：`config.go`、`source.go`、`internal/rag/loader.go`、相关测试与 `README.md`
- 受影响行为：扫描版 PDF 从“不可导入/空内容”升级为“可在配置 OCR bridge 时导入”
- 运行前提：调用方需要自行安装并配置可用的本地 OCR 命令
