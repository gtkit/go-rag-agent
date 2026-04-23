# go-rag-agent SDK Positioning Refresh Design

## Context

`go-rag-agent` 当前已经具备相对完整的 RAG runtime 能力，但仓库的对外呈现仍有三个断层：

- 首页叙事偏功能点罗列，缺少 SDK 产品定位
- 示例只覆盖一个 quickstart，无法支撑真实接入决策
- 已有 eval / trace API，但缺少可复现的基线材料和稳定性分层说明

## Goals

- 把 README 首屏重写为 SDK 产品叙事
- 用 3 个场景化示例覆盖最小接入、服务内嵌、pgvector 部署
- 用明确文档说明 API 稳定性分层
- 提供可复现的 benchmark / eval 基线文档、样例结果和生成命令
- 增加本地与 CI 共用的 release gate

## Non-Goals

- 不修改库的运行时行为
- 不新增根包 API
- 不引入 HTTP 服务端、控制台或平台化功能
- 不为示例引入新的第三方运行时依赖

## File Map

- `README.md`
  - 首屏定位、示例导航、稳定性分层、基线入口
- `VERSIONING.md`
  - 稳定性分层与升版规则补充
- `CHANGELOG.md`
  - 记录文档、示例、baseline 和 release gate 资产更新
- `examples/basic/main.go`
  - 最小接入示例
- `examples/service/main.go`
  - 服务内嵌示例
- `examples/pgvector/main.go`
  - pgvector 部署示例
- `cmd/generate-sdk-positioning-baseline/main.go`
  - 生成基线样例结果
- `internal/baselineassets/*`
  - baseline 生成逻辑与测试
- `docs/baselines/sdk-positioning.md`
  - 基线材料说明
- `scripts/verify.sh`
  - 本地与 CI 共用的 release gate
- `.github/workflows/ci.yml`
  - GitHub Actions release gate

## Decisions

### 1. 场景示例优先覆盖决策点

示例回答三个问题：

- 我怎么最快跑起来？
- 我怎么把它嵌进已有服务？
- 我怎么接 pgvector 上生产？

### 2. baseline 生成通过内部逻辑和 `cmd/` 包装

内部包负责确定性生成数据，`cmd/` 只负责参数接线和落盘路径，避免污染根包 API。

### 3. release gate 单脚本化

CI 调用 `scripts/verify.sh`，本地发布前也调用同一个脚本，避免 README / VERSIONING / CI 三处命令漂移。
