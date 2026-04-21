## 1. 结构化输出契约

- [x] 1.1 定义结构化输出结果类型与错误语义
- [x] 1.2 为 target 校验与反射结构说明补测试

## 2. JSON 提取与解析

- [x] 2.1 实现纯 JSON、code fence 和平衡 JSON 提取逻辑
- [x] 2.2 为 JSON 提取与反序列化错误补测试

## 3. 问答链接线

- [x] 3.1 为 graph 请求增加结构化输出格式约束字段
- [x] 3.2 实现 `AskStructured` / `AskStructuredWithOptions`，并复用现有同步问答链

## 4. 文档与验证

- [x] 4.1 更新 README，说明结构化输出 API 与当前限制
- [x] 4.2 更新 OpenSpec 任务状态
- [x] 4.3 运行 `openspec validate`、lint、vet、race tests
