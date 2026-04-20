## 1. OpenSpec 与根包契约

- [x] 1.1 补齐 proposal、design、specs、tasks 工件并通过 apply-ready 检查
- [x] 1.2 在根包定义 `VectorStore`、`DocumentLoader`、`Reranker` 及配套公开类型

## 2. 测试先行

- [x] 2.1 为默认兼容路径写失败测试，覆盖 `AddKnowledge`、`Ask`、`AskStream` 默认行为不变
- [x] 2.2 为组件注入写失败测试，覆盖只替换 `VectorStore`、`DocumentLoader`、`Reranker`
- [x] 2.3 为边界契约写失败测试，覆盖调用方不需要 import `internal/*`

## 3. 默认 adapter 化

- [x] 3.1 将 `ChromemStore` 通过根包默认 `VectorStore` adapter 暴露
- [x] 3.2 将当前文件加载逻辑通过根包默认 `DocumentLoader` adapter 暴露
- [x] 3.3 将当前规则重排逻辑通过根包默认 `Reranker` adapter 暴露

## 4. Agent 接线与交付

- [x] 4.1 调整 `Agent` 导入与检索编排，使其通过公开接口工作
- [x] 4.2 更新 README，说明新的默认 adapter 与组件注入用法
- [x] 4.3 更新 OpenSpec 任务状态，并运行 `openspec validate`、lint、vet、race tests
