## 1. OpenSpec 与基线确认

- [x] 1.1 运行 OpenSpec strict validation，确保 proposal/design/specs/tasks 合法
- [x] 1.2 梳理当前 `Tool`、`RuntimeComponents`、`MemoryComponents`、`TraceRecorder` 与 graph runner 接线点
- [x] 1.3 确认新增 API 不破坏现有 README stable core 和现有 tests

## 2. 结构化工具测试先行

- [x] 2.1 为结构化工具 schema 校验写失败测试，覆盖 success、required 缺失、类型错误和 enum/长度边界
- [x] 2.2 为结构化工具适配现有 `Tool`/`ToolRegistry` 写失败测试，覆盖注册、执行和旧工具兼容
- [x] 2.3 实现结构化工具类型、schema 校验、adapter 和 GoDoc

## 3. Tool-calling runner 测试先行

- [x] 3.1 为默认路径兼容写失败/回归测试，证明未启用 tool-calling 时旧路径不调用新 runner
- [x] 3.2 为 tool-calling 成功循环写失败测试，覆盖模型请求工具、执行工具、回灌结果、最终答案
- [x] 3.3 为 unknown tool、参数非法、工具失败、`MaxToolCalls`、`MaxIterations` 和 context cancel 写失败测试
- [x] 3.4 实现可选 tool-calling runner、配置校验、预算控制、context 取消、trace/callback 接线

## 4. Memory provider 测试先行

- [x] 4.1 为 memory provider retrieve 注入写失败测试，覆盖成功注入、prompt 预算裁剪和检索失败降级/报错配置
- [x] 4.2 为 memorize 写失败测试，覆盖同步问答成功、流式问答成功、失败问答不写入
- [x] 4.3 为 `LongTermMemoryStore` provider adapter 写失败测试，覆盖 TTL、去重和 session/user/tenant scope
- [x] 4.4 实现 memory provider 接口、配置字段、adapter 和 trace 记录

## 5. Trace recorder adapters 测试先行

- [x] 5.1 为 multi recorder 写失败测试，覆盖 nil 跳过、fan-out 和 panic 隔离
- [x] 5.2 为 JSONL recorder 写失败测试，覆盖可解析 JSON、摘要字段和敏感上下文不输出
- [x] 5.3 为 logger recorder 写失败测试，覆盖成功、失败和 fallback 摘要输出
- [x] 5.4 实现 recorder adapters、选项、GoDoc 和错误隔离

## 6. 示例与文档

- [x] 6.1 增加 Example tests，展示结构化工具和 trace recorder 的最小嵌入式用法
- [x] 6.2 更新 README，说明 runtime extension 使用方式、安全边界和默认关闭语义
- [x] 6.3 检查新增导出类型/函数 GoDoc 与 README 行为一致

## 7. 验证与交叉检查

- [x] 7.1 运行 `openspec validate harden-agent-runtime-extensions --type change --strict --json --no-interactive`
- [x] 7.2 运行 targeted tests 覆盖新增结构化工具、tool-calling、memory provider、trace adapter
- [x] 7.3 运行 `golangci-lint run ./...`
- [x] 7.4 运行 `go vet ./...`
- [x] 7.5 运行 `go test -race -count=1 -timeout=5m ./...`
- [x] 7.6 逐条执行代码↔需求、代码↔测试、代码↔文档、新代码↔存量代码交叉验证
