## Context

这次 change 不做运行时增强，而是修复仓库“定位表达”“接入证据”和“发布门禁”不足的问题。

## Goals / Non-Goals

**Goals:**
- 让 README 首屏更像 SDK 产品说明
- 让 `examples/` 覆盖 3 个典型场景
- 让 benchmark / eval 基线材料可复现并提交到仓库
- 让本地和 CI 使用同一套 release gate

**Non-Goals:**
- 不修改 agent、session、retrieval、storage 的行为
- 不新增根包 helper API
- 不引入服务化运行模式

## Decisions

### 1. 示例按场景切分

- `basic`: 最小接入
- `service`: 服务内嵌
- `pgvector`: 生产部署

### 2. baseline 输出必须稳定

固定知识 fixture 使用仓库内路径，report 输出中的 source path 和 chunk ID 归一化为仓库相对路径，trace summary 输出中对不稳定时长字段做归一化。

### 3. release gate 使用脚本作为唯一入口

`.github/workflows/ci.yml` 只安装必要工具并调用 `scripts/verify.sh`。验证命令的真实清单由脚本维护。

## Migration Plan

1. 补 OpenSpec artifacts
2. 实现 baseline 生成逻辑和命令
3. 改 README / VERSIONING / CHANGELOG
4. 增加示例与 baseline 文档
5. 增加 release gate 脚本与 CI workflow
6. 运行 OpenSpec 与仓库验证链
