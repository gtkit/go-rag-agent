## ADDED Requirements

### Requirement: 生产验证入口默认安全跳过
系统 SHALL 提供外部集成验证入口，用于真实 provider 和 pgvector 验收；未显式配置所需环境变量时 MUST 跳过而不是失败或伪造通过。

#### Scenario: 缺少 live 开关时跳过 provider 验证
- **WHEN** 测试环境未设置 `RAGAGENT_INTEGRATION_LIVE=1`
- **THEN** live provider integration test MUST skip，并说明需要的开关

#### Scenario: 缺少 provider 凭据时跳过 provider 验证
- **WHEN** live 开关已开启但缺少 chat 或 embedding 所需模型、base URL 或 API key
- **THEN** live provider integration test MUST skip，并列出缺失配置名

#### Scenario: 配置完整时执行真实 provider 验证
- **WHEN** live 开关和 provider 环境变量完整
- **THEN** integration test MUST 构造真实 OpenAI-compatible chat model 和 embedder，并执行一次最小 chat 与 embedding 验证

### Requirement: 生产部署模板不包含真实密钥
系统 SHALL 提供生产环境变量模板和 pgvector compose 示例，帮助操作者配置外部依赖，同时 MUST NOT 包含真实密钥、真实 DSN 或可误用的默认凭据。

#### Scenario: 环境变量模板列出必要配置
- **WHEN** 操作者查看 `.env.production.example`
- **THEN** 模板 MUST 列出 chat、embedding、pgvector、timeout、retrieval 和 observability 相关变量

#### Scenario: pgvector compose 示例可用于本地依赖验证
- **WHEN** 操作者查看 `deploy/compose/pgvector.compose.yml`
- **THEN** compose 文件 MUST 声明 PostgreSQL/pgvector 服务、healthcheck 和数据卷

#### Scenario: 模板不提交真实秘密
- **WHEN** 仓库执行敏感信息扫描
- **THEN** 模板和文档 MUST 只包含占位说明，不包含真实 API key、password、token 或私有 DSN

### Requirement: README 记录生产验收命令矩阵
系统 SHALL 在 README 或生产文档中记录本地验证、外部 provider 验证、pgvector 验证和完整提交前验证的命令矩阵。

#### Scenario: 文档区分本地验证和外部验收
- **WHEN** 操作者阅读生产验证文档
- **THEN** 文档 MUST 明确哪些命令无需外部服务，哪些命令需要显式环境变量

#### Scenario: 文档声明高风险工具边界
- **WHEN** 操作者阅读生产验证文档
- **THEN** 文档 MUST 明确 SDK 不默认内置 Shell/数据库执行/Cron 工具，生产接入这些能力需调用方自行设计授权、审计、超时和沙箱
