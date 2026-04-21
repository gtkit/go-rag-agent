## 1. 根包工具边界

- [x] 1.1 在根包定义 `Tool` 与 `ToolRegistry`
- [x] 1.2 为工具注册、重复名称和工具枚举补测试

## 2. 默认工具接线

- [x] 2.1 将默认 `retrieve_context` 与 `search_web` 接入统一注册表
- [x] 2.2 支持调用方通过配置注册或覆写工具

## 3. 工具调用上限

- [x] 3.1 让 `MaxToolCalls` 对 `retrieve_context` 与 fallback 工具链生效
- [x] 3.2 为超限停止与默认兼容路径补测试

## 4. 可观测性与文档

- [x] 4.1 保持自定义工具进入现有 telemetry / trace
- [x] 4.2 更新 README，说明 `ToolRegistry`、默认工具与 `MaxToolCalls` 语义
- [x] 4.3 更新 OpenSpec 任务状态，并运行 `openspec validate`、lint、vet、race tests
