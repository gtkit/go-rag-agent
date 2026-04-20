## ADDED Requirements

### Requirement: 仓库内维护确定性的核心回归评测集
系统 SHALL 在仓库测试中维护一个不依赖真实外部 LLM / embedding 网络调用的回归评测集，用固定 stub 锁定 runtime 注入与 trace 行为。

#### Scenario: 回归评测覆盖 runtime 注入与同步 trace
- **WHEN** 仓库执行测试套件
- **THEN** 回归评测 MUST 至少覆盖自定义 runtime 注入成功路径以及同步问答返回结构化 trace 的场景

#### Scenario: 回归评测覆盖流式 trace 终态
- **WHEN** 仓库执行测试套件
- **THEN** 回归评测 MUST 至少覆盖流式问答在终态事件中附带结构化 trace 的场景

#### Scenario: 关键行为回归时测试失败
- **WHEN** 代码变更破坏了约定的 runtime 注入、trace 结构或终态语义
- **THEN** 回归评测 MUST 以明确的用例级断言失败，而不是静默通过
