## ADDED Requirements

### Requirement: 可选的联网搜索工具
系统 SHALL 提供一个可选的联网搜索工具，首个 provider 为 Tavily，并使用外部 HTTP JSON client 完成搜索请求。

#### Scenario: 使用 Tavily 成功返回搜索结果
- **WHEN** 调用方启用了联网搜索并提供有效的 Tavily 配置
- **THEN** 系统 MUST 能对查询发起 Tavily 搜索请求，并把搜索结果格式化为可供模型消费的文本

#### Scenario: 缺少联网搜索配置时不启用工具
- **WHEN** 调用方未启用联网搜索或缺少必填配置
- **THEN** 系统 MUST 不创建联网搜索工具

#### Scenario: 联网搜索请求失败
- **WHEN** Tavily 搜索请求返回错误或超时
- **THEN** 系统 MUST 把它作为工具错误返回，而不能静默吞掉
