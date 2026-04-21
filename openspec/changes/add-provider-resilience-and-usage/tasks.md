## 1. provider 治理配置与错误模型

- [x] 1.1 定义 provider 错误分类与治理配置
- [x] 1.2 为错误分类和配置默认值补测试

## 2. provider adapter 重试/退避

- [x] 2.1 为聊天模型 adapter 增加安全重试逻辑
- [x] 2.2 为 embedding adapter 增加安全重试逻辑
- [x] 2.3 为 web search adapter 增加安全重试逻辑

## 3. usage/cost 聚合

- [x] 3.1 扩展 execution trace 的 provider usage / cost 字段
- [x] 3.2 在 provider 调用完成后汇总 usage/cost

## 4. 文档与验证

- [x] 4.1 更新 README，说明 provider 治理与当前限制
- [x] 4.2 更新 OpenSpec 任务状态
- [x] 4.3 运行 `openspec validate`、lint、vet、race tests
