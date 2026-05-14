# 生产验证与部署

本文档给出上线前验证矩阵和最小部署资产。所有凭据都必须通过环境变量、平台 secret 或密钥管理服务注入，不写入仓库。

## 环境模板

从 `.env.production.example` 开始配置运行环境。模板只列变量名和非敏感默认值，核心变量包括：

- `RAGAGENT_CHAT_MODEL`
- `RAGAGENT_CHAT_BASE_URL`
- `RAGAGENT_CHAT_API_KEY`
- `RAGAGENT_EMBEDDING_MODEL`
- `RAGAGENT_EMBEDDING_BASE_URL`
- `RAGAGENT_EMBEDDING_API_KEY`
- `RAGAGENT_PGVECTOR_DSN`
- `RAGAGENT_REQUEST_TIMEOUT_SEC`
- `RAGAGENT_TOP_K`
- `RAGAGENT_TRACE_JSONL_PATH`

## pgvector

本地或预生产 pgvector 可用 `deploy/compose/pgvector.compose.yml` 启动。先在 shell 中设置 `RAGAGENT_PGVECTOR_PASSWORD`，再运行：

```bash
docker compose -f deploy/compose/pgvector.compose.yml up -d
docker compose -f deploy/compose/pgvector.compose.yml ps
```

应用侧使用 `RAGAGENT_PGVECTOR_DSN` 指向 PostgreSQL / pgvector。集成测试使用独立的 `RAGAGENT_PGVECTOR_TEST_DSN`，避免测试写入生产库。

## 验证矩阵

每次上线前至少运行：

```bash
PATH="/Users/xiaozhaofu/go/bin:$PATH" golangci-lint run ./...
go vet ./...
go test -count=1 -cover ./...
go test -race -count=1 -timeout=5m ./...
```

真实 provider 联调默认跳过。只有在已经通过安全渠道注入 provider 环境变量后，才显式打开：

```bash
RAGAGENT_INTEGRATION_LIVE=1 go test -run TestLiveOpenAICompatibleProviders -count=1 .
```

pgvector 集成测试只在 `RAGAGENT_PGVECTOR_TEST_DSN` 存在时运行：

```bash
RAGAGENT_PGVECTOR_TEST_DSN="$RAGAGENT_PGVECTOR_TEST_DSN" go test -run TestPGVector -count=1 .
```

## 能力边界

- `Shell`：默认不开放远程命令执行工具。需要命令能力时，必须由宿主服务显式注入最小权限工具，并限制参数、超时和工作目录。
- `数据库执行`：默认不开放任意 SQL 执行工具。跨表写入必须由业务 service 编排事务，测试 DSN 与生产 DSN 分离。
- `Cron`：默认不内置调度器。后台任务由宿主平台统一管理，避免 SDK 内部产生不可观测的常驻任务。

## 上线前检查

- 配置来自环境变量或 secret，仓库中无真实凭据。
- `RAGAGENT_REQUEST_TIMEOUT_SEC` 为正整数。
- provider、pgvector、trace 路径使用不同环境隔离生产与测试。
- `golangci-lint run ./...`、`go vet ./...`、`go test -count=1 -cover ./...`、`go test -race -count=1 -timeout=5m ./...` 全部通过。
