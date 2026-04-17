## ADDED Requirements

### Requirement: 可选的详细检索与模型观测回调
系统 SHALL 提供不破坏现有 `Callback` 接口兼容性的可选观测回调，使调用方可以订阅检索、模型和 fallback 的详细运行时信息。

#### Scenario: 发出检索 metrics
- **WHEN** 调用方实现了检索 metrics 可选回调接口
- **THEN** 库 MUST 在一次检索完成后发出包含耗时、vector/lexical/fused/final 命中数、rerank shortlist 大小以及 hybrid/rerank 开关状态的检索 metrics

#### Scenario: 发出模型 metrics
- **WHEN** 调用方实现了模型 metrics 可选回调接口
- **THEN** 库 MUST 在一次模型调用完成后发出包含模型名、耗时、是否流式以及输出字符数的模型 metrics

#### Scenario: 发出 fallback 事件
- **WHEN** hybrid 或 rerank 阶段触发降级
- **THEN** 库 MUST 发出可选 fallback 事件，指出降级阶段、原错误以及退回目标
