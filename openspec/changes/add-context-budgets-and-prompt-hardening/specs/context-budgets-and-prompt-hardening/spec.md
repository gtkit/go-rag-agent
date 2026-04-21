## ADDED Requirements

### Requirement: Prompt 构造支持 token 预算治理
系统 SHALL 在现有问答链上增加 prompt token 预算治理，至少覆盖总 prompt、历史、证据和历史摘要预算。

#### Scenario: 历史按 token 预算裁剪
- **WHEN** 历史对话超过配置的历史 token 预算
- **THEN** 系统 MUST 优先保留最近轮次，而不是无限追加全部历史

#### Scenario: 旧历史会被压缩为摘要
- **WHEN** 旧历史因预算限制被裁掉
- **THEN** 系统 MUST 生成一个本地摘要文本来保留已裁掉历史的关键信息

#### Scenario: 证据文本按 token 预算裁剪
- **WHEN** 检索证据超过配置的证据 token 预算
- **THEN** 系统 MUST 在进入模型前将证据裁剪到预算范围内

### Requirement: 整轮问答支持统一执行时长预算
系统 SHALL 提供整轮执行时长预算，而不只是依赖单次外部调用的超时设置。

#### Scenario: 超出执行预算时中断问答
- **WHEN** 一次同步或流式问答的总执行时长超过配置预算
- **THEN** 系统 MUST 中断执行并返回明确的执行预算超限错误

### Requirement: 检索与工具回灌文本视为不可信输入
系统 SHALL 对检索证据和工具回灌文本做基础 prompt hardening，降低 prompt injection 风险。

#### Scenario: 检索证据按不可信上下文处理
- **WHEN** 本地检索返回证据文本
- **THEN** 系统 MUST 以“不可信上下文”语义将该文本加入 prompt，而不是直接把其中内容视为可执行指令

#### Scenario: 工具输出按不可信上下文处理
- **WHEN** fallback 工具返回文本结果
- **THEN** 系统 MUST 以与检索证据相同的不可信语义处理该结果

#### Scenario: 命中明显注入模式时进行过滤
- **WHEN** 不可信文本命中明显 prompt injection 模式
- **THEN** 系统 MUST 对这些内容做过滤或替换，而不是原样回灌
