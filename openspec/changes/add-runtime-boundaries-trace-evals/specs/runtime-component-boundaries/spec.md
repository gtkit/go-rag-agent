## ADDED Requirements

### Requirement: 根包公开稳定的运行时抽象
系统 SHALL 在根包公开稳定的聊天模型、embedding 与消息类型抽象，以及默认 OpenAI-compatible 运行时构造器，使调用方可以在不依赖 `internal/*` 的前提下实现或组合自己的 provider 适配层。

#### Scenario: 使用根包默认构造器创建聊天模型
- **WHEN** 调用方使用有效的 OpenAI-compatible 配置调用根包默认聊天模型构造器
- **THEN** 库 MUST 返回一个可供 `Agent` 使用的根包 `ChatModel` 实例

#### Scenario: 使用根包默认构造器创建 embedding 实现
- **WHEN** 调用方使用有效的 OpenAI-compatible 配置调用根包默认 embedding 构造器
- **THEN** 库 MUST 返回一个可供知识导入和检索使用的根包 `Embedder` 实例

### Requirement: Agent 构造支持运行时组件注入与混合接线
系统 SHALL 允许调用方在 Agent 配置中注入自定义 runtime 组件，并对未注入的部分继续使用文档化默认实现。

#### Scenario: 注入完整 runtime 组件时不要求默认 provider 凭据
- **WHEN** 调用方在配置中同时注入自定义 `ChatModel` 与 `Embedder`
- **THEN** 库 MUST 允许构造成功，且 MUST NOT 继续要求默认聊天或 embedding provider 配置完整

#### Scenario: 注入部分组件时保留默认路径
- **WHEN** 调用方只注入自定义 `Embedder`，并提供有效的默认聊天模型配置
- **THEN** 库 MUST 使用注入的 `Embedder` 与默认聊天模型共同构造 Agent

#### Scenario: 缺失注入组件且默认配置不完整时拒绝构造
- **WHEN** 调用方没有注入某个必需 runtime 组件，且该组件对应的默认配置也无效
- **THEN** 库 MUST 返回无效配置错误，而不是延迟到运行时才失败
