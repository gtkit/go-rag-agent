## 1. 测试先行

- [x] 1.1 为文档转换器链补失败测试，覆盖支持、跳过和失败三类路径
- [x] 1.2 为命令型转换器补失败测试，覆盖占位符、输出读取和命令失败
- [x] 1.3 为 OpenAI-compatible reranker 补失败测试，覆盖成功重排和非法响应降级输入错误
- [x] 1.4 为 MCP tool 补失败测试，覆盖成功调用和 endpoint 错误
- [x] 1.5 为 Gateway 示例补失败测试，覆盖非流式 chat completions 请求

## 2. 核心实现

- [x] 2.1 新增 `DocumentConverter` 接口、配置注入和导入链路接入
- [x] 2.2 实现命令型文档转换器
- [x] 2.3 实现 OpenAI-compatible reranker 适配器
- [x] 2.4 实现 HTTP JSON-RPC MCP tool 适配器
- [x] 2.5 新增轻量 Gateway 示例 handler

## 3. 文档与验证

- [x] 3.1 更新 README 和 Example tests，说明扩展适配能力与非控制台边界
- [x] 3.2 更新 CHANGELOG
- [x] 3.3 运行 `openspec validate`、`golangci-lint run ./...`、`go vet ./...`、`go test -race -count=1 -timeout=5m ./...`
