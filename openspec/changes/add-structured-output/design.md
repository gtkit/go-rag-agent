## Context

当前 `go-rag-agent` 已经具备：
- 同步/流式问答
- 可注入 runtime / store / reranker / tool registry
- 结构化 trace

但真正返回给调用方的结果仍然是自由文本。很多生产使用场景并不是要“看一段话”，而是要拿到一个确定 shape 的对象，例如摘要卡片、分类标签、评分、下一步动作列表等。结构化输出能力应该建立在当前 RAG 链之上，而不是要求调用方自己再做 fragile 的字符串解析。

## Goals / Non-Goals

**Goals:**
- 提供同步结构化输出 API
- 复用现有 RAG 检索、fallback、trace 和 `Answer` 语义
- 对目标结构做最小可用的反射约束，生成 JSON 输出指令
- 对模型响应做 JSON 提取和反序列化

**Non-Goals:**
- 不在这轮实现流式结构化输出
- 不接模型原生 function-calling / tool-calling JSON schema 协议
- 不替换现有 `Ask` / `AskWithOptions`

## Decisions

### 1. API 采用“方法 + target 指针”而不是泛型方法

Go 不支持在普通类型方法上声明类型参数，因此结构化输出 API 采用：
- `AskStructured(ctx, query, target any)`
- `AskStructuredWithOptions(ctx, query, opts, target any)`

其中 `target` 必须是非 nil 指针，调用方预先定义结构体并传入地址。

### 2. 结构化输出继续复用当前同步问答链

实现路径继续沿用当前：
- rewrite
- retrieve
- context assembly
- model generate

区别只在于向模型增加“JSON only”格式约束，并在得到文本后再做 JSON 提取和反序列化。

### 3. 用反射生成最小 JSON 结构说明，不引入完整 JSON Schema

第一版不引入 JSON Schema 库，也不做复杂 schema 校验。只基于反射生成足够约束模型输出的结构说明：
- struct
- slice / array
- map[string]T
- primitive

这样可以让实现保持轻量，并和当前 runtime 抽象兼容。

### 4. JSON 解析采用多阶段提取

模型返回可能是：
- 纯 JSON
- fenced code block
- 包含前后说明文字的 JSON

第一版解析按顺序尝试：
1. 直接 JSON 反序列化
2. 剥离 code fence
3. 提取第一个平衡的 JSON object / array

如果仍失败，返回明确错误。

## Risks / Trade-offs

- [模型不完全遵守 JSON 指令] → 增加多阶段 JSON 提取，而不是只依赖 strict raw output
- [反射生成的结构说明不够完整] → 第一版只追求 struct/slice/map/primitive 的最小可用支持
- [结构化输出和自由文本 API 行为交叉] → 新能力单独暴露，不改旧接口

## Migration Plan

1. 先补 proposal/design/spec/tasks
2. 先写失败测试，锁结构化输出 API、JSON 提取和 Ask 主流程接线
3. 新增结构化输出结果类型、错误和反射辅助
4. 给 graph 请求增加格式约束字段，并接线到 `ChatRunner`
5. 更新 README、任务状态并跑完整验证

## Open Questions

- 无。流式结构化输出和模型原生 function-calling 明确排除在本 change 外。
