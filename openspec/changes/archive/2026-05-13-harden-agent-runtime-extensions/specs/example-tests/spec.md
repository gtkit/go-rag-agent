## ADDED Requirements

### Requirement: 示例覆盖 runtime extension 的嵌入式用法
系统 SHALL 通过 examples 或 Example tests 展示结构化工具、显式 tool-calling、memory provider adapter 和 trace recorder adapter 的最小嵌入式用法。

#### Scenario: 结构化工具示例可编译
- **WHEN** 仓库执行 example tests
- **THEN** 结构化工具示例 MUST 编译通过，并展示 schema、参数校验和适配到 `ToolRegistry` 的用法

#### Scenario: Trace recorder 示例可编译
- **WHEN** 仓库执行 example tests
- **THEN** trace recorder adapter 示例 MUST 编译通过，并展示 JSONL 或 logger recorder 的最小用法

#### Scenario: README 明确安全边界
- **WHEN** 用户阅读 README 中 runtime extension 章节
- **THEN** 文档 MUST 明确 tool-calling 默认关闭，SDK 不内置 Shell/数据库执行工具，生产环境工具应由调用方自行做授权、超时和审计
