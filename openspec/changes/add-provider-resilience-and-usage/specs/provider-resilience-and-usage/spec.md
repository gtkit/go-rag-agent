## ADDED Requirements

### Requirement: provider 调用具备统一错误分类
系统 SHALL 对聊天模型、embedding 和联网搜索调用失败建立统一错误分类，至少区分 rate limit、auth、transient、permanent。

#### Scenario: 429 被识别为 rate limit
- **WHEN** provider 返回 429 或等价的限流信号
- **THEN** 系统 MUST 将其归类为 rate limit 错误

#### Scenario: 401/403 被识别为 auth
- **WHEN** provider 返回 401、403 或等价认证失败信号
- **THEN** 系统 MUST 将其归类为 auth 错误

### Requirement: 安全 provider 调用支持有限次重试与退避
系统 SHALL 为安全的 provider 调用提供有限次重试与退避，而不对所有路径盲目重试。

#### Scenario: embedding 调用遇到瞬时错误时可重试
- **WHEN** embedding 调用遇到瞬时网络错误、超时或可重试的 5xx
- **THEN** 系统 MUST 在预算范围内按退避策略重试

#### Scenario: web search 调用遇到瞬时错误时可重试
- **WHEN** web search 调用遇到瞬时网络错误、超时或可重试的 5xx
- **THEN** 系统 MUST 在预算范围内按退避策略重试

#### Scenario: 流式聊天在输出首个 chunk 后不再自动重试
- **WHEN** 流式聊天已经输出了至少一个 chunk 后再失败
- **THEN** 系统 MUST 直接返回错误，而不是自动重试

### Requirement: execution trace 聚合 provider usage 与 estimated cost
系统 SHALL 在 execution trace 中记录 provider 调用摘要、usage 和 estimated cost。

#### Scenario: provider 返回 usage 时聚合到 trace
- **WHEN** provider 返回 token usage 信息
- **THEN** 系统 MUST 将 usage 聚合到 execution trace

#### Scenario: provider 未返回 usage 时不强制填充
- **WHEN** provider 不返回 usage 信息
- **THEN** 系统 MAY 保持对应 usage/cost 字段为 0，但仍要记录 provider 调用摘要
