## ADDED Requirements

### Requirement: 未使用且来源不稳定的直接依赖不得保留
系统 SHALL 不保留仅靠测试空白导入维持、且当前仓库未实际使用的直接依赖，特别是在该依赖版本对 proxy 与源端可见性不一致时。

#### Scenario: 删除仅由测试空白导入保留的 logger 依赖
- **WHEN** `github.com/gtkit/logger` 只因测试空白导入被保留，且仓库中不存在实际 API 使用
- **THEN** 系统 MUST 删除该直接依赖及对应空白导入，而不是继续保留它

#### Scenario: 不为未使用依赖做主版本升级
- **WHEN** 一个依赖当前未被仓库实际使用，但存在更新的主版本模块路径
- **THEN** 系统 MUST 优先删除该依赖，而不是为了占位把它升级到新的主版本
