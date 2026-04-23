## Why

当前仓库已经具备较完整的 RAG runtime 能力，但定位资产仍然偏弱：

- README 首屏更像能力罗列，不像 SDK 产品说明
- `examples/` 只有单一 quickstart，无法覆盖典型接入场景
- eval / trace 已有 API，却没有可复现的基线材料入口
- 发布前验证命令散落在文档中，缺少本地与 CI 共用的 release gate

这会影响新使用者对项目定位、接入方式和可信度的判断。

## What Changes

- 重写 README 顶部定位与示例导航
- 增加 3 个场景化示例
- 补 API 稳定性分层说明
- 增加 benchmark / eval 基线文档、样例结果和生成命令
- 增加 `scripts/verify.sh` 与 GitHub Actions CI release gate

## Capabilities

### New Capabilities
- `sdk-positioning-assets`: 定义仓库级定位文档、场景化示例、可复现 baseline 资产和 release gate

### Modified Capabilities
- 无

## Impact

- 受影响文件：`README.md`、`VERSIONING.md`、`CHANGELOG.md`、`examples/*`、`docs/baselines/*`、`cmd/generate-sdk-positioning-baseline/*`、`scripts/verify.sh`、`.github/workflows/ci.yml`
- 不改变根包运行时行为和公开 API 语义
