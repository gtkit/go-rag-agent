## 1. 预算与安全配置

- [x] 1.1 在 `Config` 中增加 prompt 预算、执行时长预算和 prompt hardening 配置
- [x] 1.2 为这些配置的默认值与校验规则补测试

## 2. Prompt 治理实现

- [x] 2.1 实现历史 token 预算裁剪与本地摘要压缩
- [x] 2.2 实现证据 token 预算裁剪
- [x] 2.3 实现检索/工具文本的 prompt hardening

## 3. 执行预算接线

- [x] 3.1 为同步问答接入整轮执行时长预算
- [x] 3.2 为流式问答接入整轮执行时长预算
- [x] 3.3 为预算超限补测试

## 4. 文档与验证

- [x] 4.1 更新 README，说明上下文治理与提示硬化配置
- [x] 4.2 更新 OpenSpec 任务状态
- [x] 4.3 运行 `openspec validate`、lint、vet、race tests
