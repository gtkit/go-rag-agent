## Context

当前 `Callback` 只暴露基础生命周期：
- OnRetrieveStart / End
- OnToolStart / End
- OnModelStart / End

这对“功能是否执行”足够，但对企业级线上运行不够，因为还缺：
- 检索耗时、模型耗时
- hybrid vector/lexical/fused 命中数
- rerank shortlist 大小
- 是否发生 fallback、发生在哪个阶段

同时，当前 hybrid/rerank 已经是多阶段检索链路，但内部如果出现异常，没有明确的阶段性降级策略。

## Goals / Non-Goals

**Goals：**

- 在不破坏现有 `Callback` 接口兼容性的前提下，提供更细粒度观测
- 为 hybrid 失败提供 vector-only fallback
- 为 rerank 失败提供 hybrid fallback
- 让 fallback 事件对调用方可见

**Non-Goals：**

- 不接入外部 tracing/metrics SDK
- 不引入新的日志框架绑定
- 不改变现有问答 API

## Decisions

### 1. 新增可选回调接口，而不是扩展现有 Callback

选择：
- 保留现有 `Callback` 原样
- 额外定义可选接口：
  - `RetrievalMetricsCallback`
  - `ModelMetricsCallback`
  - `FallbackCallback`

原因：
- 向后兼容
- 调用方按需实现，不增加已有实现的编译负担

### 2. 详细观测直接在根包层发出

选择：
- 现有 `internal/telemetry.Dispatcher` 继续负责基础生命周期 fan-out
- 详细 metrics / fallback 事件由 `agent.go` 对 `Config.Callbacks` 做可选接口断言后发出

原因：
- `internal` 包不能被外部调用方引用
- 根包最适合承载对外观测契约

### 3. fallback 只对增强阶段生效

选择：
- hybrid 路径失败时回退到 vector-only
- rerank 路径失败时回退到 hybrid fused hits
- 向量检索本身失败时仍返回错误，不做假降级

原因：
- fallback 的目标是保住“本来还能回答”的场景
- 如果基础向量检索都失败，已经没有可靠证据来源

### 4. metrics 以请求级聚合结果为主

选择：
- `RetrievalMetrics` 提供请求级聚合字段：
  - duration
  - hybrid/rerank enabled
  - vector/lexical/fused/final hit counts
  - rerank shortlist size
- `ModelMetrics` 提供：
  - model
  - duration
  - stream
  - output chars

原因：
- 足够覆盖企业级线上观测的第一层需求
- 比逐阶段细粒度 trace 更容易稳定落地

## Risks / Trade-offs

- [观测接口增多使 API 面变大] → 只增加可选接口，不改现有基础接口
- [fallback 隐藏真实问题] → 明确发出 `FallbackEvent`，让调用方能记录和报警
- [过多观测字段增加实现复杂度] → 首版只保留请求级关键指标

## Migration Plan

1. 定义新的可选回调接口与事件结构
2. 在检索链路中补 metrics 采集和 fallback 发出
3. 在模型路径补 model metrics
4. 增加测试与 README

## Open Questions

无
