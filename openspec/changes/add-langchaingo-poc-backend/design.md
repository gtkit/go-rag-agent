## Context

当前库已经把知识导入、向量检索、会话记忆和联网搜索封装成内部独立模块，但问答执行路径仍直接依赖 Eino 的 `ToolCallingChatModel`、工具协议和 ReAct agent。结果是 LLM provider 适配层虽然较薄，执行编排层却与 Eino 类型深度耦合，后续若要减小依赖面、替换后端或做双后端对比，需要同时改模型、工具和 graph 实现。

本次变更只在实验分支中验证一个 LangChainGo 执行后端，要求保留现有公开 API 和调用方式，并继续支持两条核心能力：本地知识库搜索与 Tavily 联网搜索。

## Goals / Non-Goals

**Goals:**
- 用 LangChainGo 重写内部 graph 执行后端，保持 `Agent` / `Session` 外部 API 不变
- 保持本地检索工具和联网搜索工具可被问答流程调用
- 保持同步问答和流式问答两条路径可用
- 用测试锁定本地检索优先、证据不足时可联网搜索的核心行为

**Non-Goals:**
- 不新增对外公开配置项或重新设计根包 API
- 不重写知识导入、向量存储、embedding、会话记忆等既有模块
- 不追求与 Eino 在所有内部执行细节上逐字节一致
- 不在这次 PoC 中做双后端运行时切换

## Decisions

### 1. 保留现有 `graph.Runner` 抽象，替换其内部实现

直接在 `internal/graph` 下引入 LangChainGo runner，实现 `Ask` / `AskStream`，而不是把 LangChainGo 类型向上冒泡。这样 `agent.go` 和 `Session` 仍依赖本地接口，外部 API 无需变化。

备选方案：
- 继续使用 Eino，仅做更薄的 wrapper。否决原因：不能验证替换后端是否成立。
- 在根包增加后端选择配置。否决原因：PoC 阶段会扩大公开 API 面。

### 2. 将工具层从 Eino tool 适配改为本地工具契约

本地检索和联网搜索工具目前只在 `internal/tools` 中做参数解析与结果格式化，业务逻辑已经分别落在 `Retriever` 与 `websearch.Searcher` 接口后面。PoC 中把工具适配层收敛成项目内自定义工具契约，再由 LangChainGo agent executor 绑定。

备选方案：
- 继续让 `internal/tools` 直接暴露第三方框架类型。否决原因：会把新的框架耦合重复到工具层。

### 3. 保留现有 OpenAI-compatible chat / embedding 配置结构，但聊天实现改用 LangChainGo OpenAI LLM

embedding 已经通过本地 `Embedder` 接口隔离，可以暂时维持现状；聊天模型改由 LangChainGo 提供的 OpenAI-compatible 客户端实现，以移除 Eino 对聊天与工具调用的绑定。

备选方案：
- 连 embedding 一并切到 LangChainGo。否决原因：本次目标是执行后端验证，扩大改动会增加变量。

### 4. 流式路径以项目内事件协议为准，不暴露 LangChainGo 原生流式对象

`graph.Event` 已经是项目内协议。PoC 里继续把 LangChainGo 的 token/step 输出转换成 `answer_chunk` 和 `done`，保持上层 Session 不感知底层流实现。

## Risks / Trade-offs

- [LangChainGo agent API 与当前测试假设不完全一致] → 先写失败测试锁定本地检索和联网搜索能力，只要求满足库现有行为合同
- [第三方依赖升级导致编译面扩大] → 将 LangChainGo 使用面限制在 `internal/graph` 与 `internal/llm`，避免向其他层扩散
- [流式能力实现细节不如 Eino 直接] → 优先保证同步路径和核心流式分块事件正确，再验证边缘流式语义
- [PoC 分支与主线出现文档偏差] → README 明确标注该分支为 LangChainGo PoC 实现

## Migration Plan

1. 在实验分支中引入 LangChainGo 依赖并保留现有公开 API
2. 先补充 runner 与工具层测试，验证本地检索与联网搜索路径
3. 切换 `agent.go` 到新 runner 和新工具契约
4. 更新 README 与 OpenSpec 任务状态
5. 运行 OpenSpec validate、lint、vet、race tests
6. push 实验分支，作为后续对照基线

## Open Questions

- 无。PoC 范围固定为单后端替换，并以现有公开 API 与核心搜索能力为验收标准。
