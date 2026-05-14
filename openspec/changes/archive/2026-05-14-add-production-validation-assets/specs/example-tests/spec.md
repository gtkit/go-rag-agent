## MODIFIED Requirements

### Requirement: 库提供 Example tests
系统 SHALL 提供根包 Example tests，覆盖常见组合能力，并 SHALL 提供可测试的生产验证示例入口。

#### Scenario: Example 覆盖 basic 用法
- **WHEN** 用户阅读根包 Example tests
- **THEN** 系统 MUST 提供 basic 用法示例

#### Scenario: Example 覆盖两级 prompt cache 和 eval runner
- **WHEN** 用户阅读根包 Example tests
- **THEN** 系统 MUST 提供 two-level prompt cache 和 eval runner 示例

#### Scenario: 生产验证示例可被本地测试检查
- **WHEN** 仓库执行普通测试
- **THEN** 生产验证示例的 env parsing、skip reason 和部署文件内容 MUST 被本地测试覆盖，而不需要真实外部服务
