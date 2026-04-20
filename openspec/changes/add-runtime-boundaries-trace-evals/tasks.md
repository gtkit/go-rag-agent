## 1. OpenSpec 与接口准备

- [x] 1.1 补齐 proposal、design、specs、tasks 工件并通过 apply-ready 检查
- [x] 1.2 在根包公开 runtime 抽象、默认构造器与配置注入字段

## 2. 测试先行

- [x] 2.1 为 runtime 组件注入写失败测试，覆盖完整注入、混合接线与缺失默认配置
- [x] 2.2 为同步问答 trace 与 recorder/logger 接线写失败测试
- [x] 2.3 为流式问答终态 trace 写失败测试

## 3. 运行时实现

- [x] 3.1 调整 `Agent.New`、默认 provider 构造与校验逻辑，支持稳定 runtime 边界
- [x] 3.2 实现结构化 execution trace、trace recorder 和 logger 摘要输出
- [x] 3.3 在 graph 层补齐 web tool trace 观测并保持现有问答语义

## 4. 回归评测与文档

- [x] 4.1 增加确定性 regression suite，锁定 runtime 注入与 trace 行为
- [x] 4.2 更新 README，说明 runtime 注入、trace 与回归测试用法
- [x] 4.3 更新 OpenSpec 任务状态，并运行 `openspec validate`、lint、vet、race tests
