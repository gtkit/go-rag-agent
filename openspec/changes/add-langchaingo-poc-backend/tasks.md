## 1. OpenSpec 与依赖准备

- [x] 1.1 在实验分支中补齐 proposal、design、specs 工件并通过 apply-ready 检查
- [x] 1.2 引入 LangChainGo 依赖并移除 Eino 执行后端相关直接依赖

## 2. 测试先行

- [x] 2.1 为同步问答补充失败测试，覆盖本地知识检索优先
- [x] 2.2 为同步问答补充失败测试，覆盖证据不足时可进入联网搜索
- [x] 2.3 为流式问答补充失败测试，覆盖搜索语义保持一致

## 3. 执行后端替换

- [x] 3.1 重写内部工具契约，解耦 Eino tool 类型
- [x] 3.2 用 LangChainGo 实现新的 graph runner
- [x] 3.3 调整 `agent.go` 与 `internal/llm` 接线，保持公开 API 不变

## 4. 交付与验证

- [x] 4.1 更新 README，说明 LangChainGo PoC 分支能力与限制
- [x] 4.2 更新 OpenSpec 任务完成状态并通过 `openspec validate`
- [x] 4.3 运行 lint、vet、race tests，并 push 分支到 GitHub
