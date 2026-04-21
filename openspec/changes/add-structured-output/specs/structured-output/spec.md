## ADDED Requirements

### Requirement: 同步问答支持结构化输出
系统 SHALL 提供同步结构化输出 API，使调用方可以提供目标结构，并获得已反序列化的结果，而不改变现有 `Ask` / `AskWithOptions` 入口。

#### Scenario: 调用方通过目标结构接收 JSON 输出
- **WHEN** 调用方向结构化输出 API 传入有效的目标结构指针
- **THEN** 库 MUST 生成同步问答结果，并把模型返回的 JSON 反序列化到该目标结构中

#### Scenario: 结构化输出继续返回原始问答结果信息
- **WHEN** 一次结构化输出请求执行成功
- **THEN** 调用方 MUST 仍然能够获取原始问答文本、引用和 trace，而不只是反序列化后的对象

### Requirement: 结构化输出目标必须显式可写
系统 SHALL 校验结构化输出目标，避免对无效 target 执行不确定行为。

#### Scenario: target 不是非 nil 指针时返回错误
- **WHEN** 调用方向结构化输出 API 传入非指针、nil 指针或不可写目标
- **THEN** 库 MUST 返回明确错误，而不是 panic

#### Scenario: 结构化说明覆盖常见 Go 形状
- **WHEN** target 是 struct、slice、array、`map[string]T` 或基础类型
- **THEN** 库 MUST 能为该类型生成足够约束模型输出的 JSON 结构说明

### Requirement: 模型响应经过 JSON 提取后再反序列化
系统 SHALL 对模型文本响应执行 JSON 提取，再反序列化到目标结构。

#### Scenario: 纯 JSON 可直接反序列化
- **WHEN** 模型返回纯 JSON 文本
- **THEN** 库 MUST 直接将其反序列化到目标结构

#### Scenario: fenced code block 中的 JSON 仍可解析
- **WHEN** 模型把 JSON 包在 Markdown code fence 中返回
- **THEN** 库 MUST 提取其中的 JSON 并完成反序列化

#### Scenario: 包含额外说明文字时尝试提取首个平衡 JSON
- **WHEN** 模型返回文本前后带说明，但中间包含一个可解析的 JSON object 或 array
- **THEN** 库 MUST 提取该 JSON 并完成反序列化

#### Scenario: 无法解析时返回明确错误
- **WHEN** 模型响应最终无法提取并反序列化为 JSON
- **THEN** 库 MUST 返回明确错误，而不是静默返回零值结构
