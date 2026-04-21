## Context

当前 provider 调用链主要有三类：
- 聊天模型（OpenAI-compatible）
- embedding 模型（OpenAI-compatible）
- Tavily web search

这些调用已经有单次请求超时，但缺少：
- 对 `429` / 瞬时网络错误 / 5xx 的统一分类
- bounded retries with backoff
- usage/cost 聚合

因此下一步的优先级不应该先做断路器，而应该先把“错误分类 + 重试/退避 + usage/cost 聚合”这三件事补齐。

## Goals / Non-Goals

**Goals:**
- 为 provider 错误建立统一分类
- 为安全的 provider 调用增加有限次重试与退避
- 在 execution trace 中记录 provider usage / estimated cost
- 保持现有 API 与默认行为兼容

**Non-Goals:**
- 不在这轮实现 provider 限流
- 不在这轮实现断路器
- 不做复杂多 provider 策略调度

## Decisions

### 1. provider 治理主要落在 adapter 层

聊天、embedding 和 web search 的具体治理逻辑主要放在 provider adapter 层，而不是高层 `Agent`。`Agent` 只负责汇总 trace 与 cost。

### 2. 重试只用于幂等或安全路径

第一版重试范围：
- `EmbedTexts`
- web search
- `Generate`（仅同步问答）
- `Stream` 只在“尚未输出任何 chunk”时允许重试

不对已输出部分流式结果的 provider 调用做盲重试。

### 3. usage/cost 聚合通过 provider trace 实现

execution trace 扩展 provider 调用摘要：
- provider
- operation
- attempts
- error class
- usage tokens
- estimated cost

如果底层 provider 未返回 usage，则允许字段为 0。

## Risks / Trade-offs

- [流式重试边界复杂] → 第一版只在未输出 chunk 时允许重试
- [某些 provider 不返回 usage] → 保持 usage/cost 可选，不强求所有 provider 完整填充
- [默认开启重试会改变时延] → 使用小而保守的默认 attempts 和 backoff

## Migration Plan

1. 补 proposal/design/spec/tasks
2. 先写失败测试，锁错误分类、重试和 usage 聚合
3. 在 provider adapter 层实现错误分类与重试
4. 扩展 trace 和 README
5. 跑完整验证

## Open Questions

- 无。限流和断路器明确后置到后续 change。
