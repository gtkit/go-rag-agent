# go-rag-agent

`go-rag-agent` 是一个基于 Go 1.26.2 的本地优先 RAG 运行时，提供：
- 本地知识导入
- chromem 向量检索
- 会话记忆
- OpenAI-compatible 聊天 / embedding 适配
- 可注入的 runtime 组件边界
- 单次执行结构化 trace

## 安装

当前模块路径是 `github.com/gtkit/go-rag-agent`。

推荐在本地工作区或内部代码仓中引用它，而不是按公网模块直接 `go get`。

示例：

```go
require github.com/gtkit/go-rag-agent v0.0.0

replace github.com/gtkit/go-rag-agent => ../go-rag-agent
```

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

func main() {
	ctx := context.Background()

	cfg := ragagent.Config{
		ChatModel:      "gpt-4o-mini",
		ChatBaseURL:    "https://api.openai.example/v1",
		ChatAPIKey:     "replace-with-your-chat-key",
		EmbeddingModel: "text-embedding-3-small",
		EmbeddingAPIKey:"replace-with-your-embedding-key",
		DataDir:        ".rag-data",
		RequestTimeout: 20 * time.Second,
	}

	agent, err := ragagent.New(cfg)
	if err != nil {
		log.Fatalf("new agent: %v", err)
	}
	defer func() { _ = agent.Close() }()

	knowledgeDir, err := os.MkdirTemp("", "ragagent-knowledge-*")
	if err != nil {
		log.Fatalf("make temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(knowledgeDir) }()

	knowledgeFile := filepath.Join(knowledgeDir, "architecture.md")
	content := []byte("# Architecture\n\nThis knowledge base is loaded from a temp directory.")
	if err := os.WriteFile(knowledgeFile, content, 0o600); err != nil {
		log.Fatalf("write temp knowledge: %v", err)
	}

	if err := agent.AddKnowledge(ctx, ragagent.DirSource(knowledgeDir)); err != nil {
		log.Fatalf("add knowledge: %v", err)
	}

	answer, err := agent.GetSession("demo").Ask(ctx, "Summarize the architecture in knowledge/")
	if err != nil {
		log.Fatalf("ask: %v", err)
	}
	fmt.Println(answer.Text)
}
```

## 配置说明

必填项：
默认 provider 路径下必填：
- `ChatModel`
- `ChatBaseURL`
- `ChatAPIKey`
- `EmbeddingModel`

可选项（由 `New` / `Validate` 应用默认值）：
- `TopK`（默认 `5`）
- `ChunkSize`（默认 `1000`）
- `MaxHistoryRounds`（默认 `8`）
- `MaxIterations`（默认 `3`，当前 LangChainGo PoC 分支里保留字段但未参与内部执行循环）
- `RequestTimeout`（默认 `30s`）
- `EnableHybridSearch`（默认 `false`）
- `EnableRerank`（默认 `false`）
- `EnableWebSearch`（默认 `false`）
- `HybridCandidateMultiplier`（默认 `4`）
- `HybridRRFK`（默认 `60`）
- `RerankShortlistMultiplier`（默认 `2`）
- `Runtime`
  说明：可选 runtime 注入；支持注入 `ChatModel`、`Embedder`
- `Storage`
  说明：可选存储/加载/rerank 注入；支持注入 `VectorStore`、`DocumentLoader`、`Reranker`
- `TraceRecorder`
  说明：可选单次执行 trace sink
- `Logger`
  说明：可选日志实例；只要实现 `Debug/Info/Warn/Error(msg string, kv ...any)` 即可

校验说明：
- 空 `DataDir` 表示使用内存模式，不会强制写入当前目录。
- 当 `Runtime.ChatModel` 已注入时，默认聊天 provider 配置可省略。
- 当 `Runtime.Embedder` 已注入时，默认 embedding provider 配置可省略。
- 当 `Storage` 里某个组件已注入时，该组件会覆盖默认 adapter；未注入部分继续使用默认实现。
- `SimilarityThreshold: 0` 会保留非负相似度结果；如果你希望连负相似度结果也保留，需要传负值。
- `ChunkSize` 必须不超过当前证据拼装预算（`<= 4000` rune）。
- `ChunkOverlap` 必须满足 `>= 0` 且 `< ChunkSize`。
- `EnableRerank` 只能在 `EnableHybridSearch=true` 时启用。
- `EnableWebSearch=true` 时必须提供有效的 `WebSearch.APIKey`。
- `HybridCandidateMultiplier` 必须是正数。
- `HybridRRFK` 必须是正数。
- `RerankShortlistMultiplier` 必须是正数。
- `MaxToolCalls` 目前在 Phase 1 里保留字段，但还没有真正接入运行时控制。
- `PDFOCRBridge` 只有在你要导入扫描版 PDF 时才需要配置；如果配置了，`Args` 必须同时包含 `{input}` 和 `{output}` 占位符。

## 自定义 runtime 注入

如果你不想直接使用库内默认的 OpenAI-compatible runtime，可以通过 `Config.Runtime` 注入自定义组件。

库同时公开了默认构造器：
- `NewOpenAIChatModel(ctx, cfg)`
- `NewOpenAIEmbedder(ctx, cfg)`

你可以选择三种接线方式：
- 全部使用默认 runtime
- 全部注入自定义 runtime
- 部分注入，其余部分继续走默认构造

示例：

```go
chatModel, err := ragagent.NewOpenAIChatModel(ctx, ragagent.ChatModelConfig{
	Model:   "gpt-4o-mini",
	BaseURL: "https://api.openai.example/v1",
	APIKey:  "replace-with-your-chat-key",
	Timeout: 20 * time.Second,
})
if err != nil {
	log.Fatalf("new chat model: %v", err)
}

embedder, err := ragagent.NewOpenAIEmbedder(ctx, ragagent.EmbedderConfig{
	Model:   "text-embedding-3-small",
	BaseURL: "https://api.openai.example/v1",
	APIKey:  "replace-with-your-embedding-key",
	Timeout: 20 * time.Second,
})
if err != nil {
	log.Fatalf("new embedder: %v", err)
}

cfg := ragagent.Config{
	Runtime: ragagent.RuntimeComponents{
		ChatModel: chatModel,
		Embedder:  embedder,
	},
}
```

如果你有自己的 provider 抽象层，只要实现根包公开的 `ChatModel` / `Embedder` 接口即可。

## 存储、加载与重排边界

当前库也支持通过 `Config.Storage` 注入存储、文档加载和 rerank 组件：

- `VectorStore`
- `DocumentLoader`
- `Reranker`

默认不传时分别使用：
- `NewChromemVectorStore(...)`
- `NewFileDocumentLoader()`
- `NewRuleBasedReranker()`

这样可以只替换其中一部分，而不用改 `AddKnowledge`、`Ask`、`AskStream` 这些公开入口。

示例：

```go
store, err := ragagent.NewChromemVectorStore(ragagent.ChromemVectorStoreConfig{
	DataDir:    ".rag-data",
	Collection: "knowledge",
})
if err != nil {
	log.Fatalf("new vector store: %v", err)
}

cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    "https://api.openai.example/v1",
	ChatAPIKey:     "replace-with-your-chat-key",
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey:"replace-with-your-embedding-key",
	Storage: ragagent.StorageComponents{
		VectorStore:    store,
		DocumentLoader: ragagent.NewFileDocumentLoader(),
		Reranker:       ragagent.NewRuleBasedReranker(),
	},
}
```

如果你有自己的向量库、文档加载器或重排器，只要实现根包公开的接口即可。默认行为不变，只有你显式注入的部分会被覆盖。

## 知识导入流程

1. 通过 `FileSource(path)` 或 `DirSource(path)` 提供来源。
   当前支持：
   - `.txt`
   - `.md`
   - 文本型 `.pdf`
   - 扫描版 `.pdf`（配置 OCR bridge 后）
2. `AddKnowledge` 会先解析文件，再加载为 RAG 文档。
3. 文本按 rune 窗口进行切块（`ChunkSize`、`ChunkOverlap`）。
4. 每个 chunk 通过 embedding 适配器向量化。
5. 结果写入本地 chromem 存储。

目录同步语义：
- `DirSource(path)` 成功重导入同一路径时，会以当前目录里的受支持文件集合为准。
- 如果某些文件上一次导入后已经从该目录删除，本次成功导入会把这些文件对应的历史索引分块一起删除。
- 如果目录已经变空，本次成功导入会清空该目录上一次成功导入留下的历史索引。
- 设置 `DataDir` 后，这个目录删除同步状态会跨 Agent 重启保留。
- `FileSource(path)` 仍然只处理单文件；有道笔记桥接导入也不参与目录级删除同步。

说明：
- 文本型 PDF 会优先直接抽取文本。
- 当 PDF 无法直接抽取可用文本时，如果配置了 `PDFOCRBridge`，会自动走 OCR fallback。
- 如果扫描版 PDF 没有配置 `PDFOCRBridge`，导入会返回明确错误，不会静默导入空内容。
- Markdown 支持 YAML front matter，导入时会自动提取为 metadata，并从正文中剥离该 front matter。
- 所有支持的知识文件都支持 sidecar metadata，命名规则是 `<basename>.meta.json|yaml|yml`。
- 如果同一个 Markdown 同时存在 front matter 和 sidecar metadata，sidecar 的同名字段会覆盖 front matter。

## 扫描版 PDF / OCR

如果你的知识库里有扫描版 PDF、图片型 PDF，可以在 `Config` 里配置 `PDFOCRBridge`。

设计约束是：
- 库本身不绑定某个 OCR SDK 或云服务
- 你提供本地 OCR 命令
- 命令读取 `{input}` 指向的 PDF，并把识别后的纯文本写入 `{output}` 指向的文本文件

示例：

```go
cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    "https://api.openai.example/v1",
	ChatAPIKey:     "replace-with-your-chat-key",
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey:"replace-with-your-embedding-key",
	PDFOCRBridge: ragagent.PDFOCRBridgeConfig{
		Command: "my-pdf-ocr",
		Args: []string{
			"{input}",
			"{output}",
		},
	},
}
```

你可以把它接到自己的包装脚本，或者系统里已有的 OCR 工具链。当前库只约定 bridge 契约，不强制具体供应商。

可选项：
- `MinDirectTextRunes`
  说明：当直接抽取出来的 PDF 文本 rune 数低于这个值时，也会走 OCR fallback。默认 `0`，表示只在“完全提不出可用文本”时才 OCR。

限制：
- OCR 结果质量取决于你配置的本地 OCR 工具和语言包。
- 当前只接收 OCR bridge 输出的纯文本，不做版面分析、表格结构恢复或富文本重建。

## 过滤检索

当知识库里有多个目录、多个来源，或者你只想让问题限定在某类文档里时，可以使用带选项的查询接口：

- `AskWithOptions(ctx, query, opts)`
- `AskStreamWithOptions(ctx, query, opts, emit)`

过滤条件放在 `QueryOptions.Filter` 中，当前支持：
- `SourcePaths`
  说明：精确文件路径匹配
- `SourcePrefixes`
  说明：按来源路径前缀限制，适合限定到某个目录树
- `Metadata`
  说明：精确元数据匹配；多个键值会按 AND 关系同时生效

示例：

```go
opts := ragagent.QueryOptions{
	Filter: ragagent.RetrievalFilter{
		SourcePrefixes: []string{
			"/Users/me/knowledge/backend",
		},
		Metadata: map[string]string{
			"tag": "api",
		},
	},
}

answer, err := agent.GetSession("me").AskWithOptions(ctx, "总结网关接口规范", opts)
if err != nil {
	log.Fatalf("ask with options: %v", err)
}
fmt.Println(answer.Text)
```

流式接口用法相同：

```go
err := agent.GetSession("me").AskStreamWithOptions(ctx, "总结网关接口规范", opts, func(event ragagent.StreamEvent) error {
	// 处理流式事件
	return nil
})
```

当前限制：
- `Metadata` 只支持精确匹配，不支持模糊匹配和范围查询。
- `SourcePrefixes` 适合目录级限制；如果你要精确锁定单个文件，请优先用 `SourcePaths`。
- 如果过滤后没有可用证据，接口会返回证据不足错误，不会回退到全库检索。

## Hybrid Retrieval 与 Rerank

当前库支持两级检索增强：

- `EnableHybridSearch`
  说明：把纯向量召回升级为“向量召回 + lexical recall”融合。
- `EnableRerank`
  说明：在 hybrid shortlist 上执行可选重排。
- `HybridCandidateMultiplier`
  说明：控制 hybrid 路径保留多少倍 `TopK` 的候选参与融合。默认 `4`。
- `HybridRRFK`
  说明：控制 RRF 融合时排名衰减强度。默认 `60`。
- `RerankShortlistMultiplier`
  说明：控制 rerank 作用的 shortlist 大小。默认 `2`，即约 `TopK * 2`。

示例：

```go
cfg := ragagent.Config{
	ChatModel:         "gpt-4o-mini",
	ChatBaseURL:       "https://api.openai.example/v1",
	ChatAPIKey:        "replace-with-your-chat-key",
	EmbeddingModel:    "text-embedding-3-small",
	EmbeddingAPIKey:   "replace-with-your-embedding-key",
	EnableHybridSearch: true,
	EnableRerank:       true,
	HybridCandidateMultiplier: 4,
	HybridRRFK:                60,
	RerankShortlistMultiplier: 2,
}
```

行为说明：
- hybrid retrieval 会把精确词项命中和语义相关性一起纳入排序。
- rerank 首版只在 shortlist 上工作，不会对全量候选做重排。
- 同步问答、流式问答和内部 retrieval tool 路径会统一使用同一套检索策略。

适用场景：
- 接口名、表名、缩写、文件名这类 lexical 信号很强的查询
- 纯向量召回容易把“语义相关但词项不精确”的内容排在前面时

当前限制：
- lexical recall 首版采用本地简单 tokenizer 和候选扫描策略，不是完整全文检索引擎。
- rerank 首版是本地规则实现，没有接入额外模型服务。

推荐上线默认值：
- 大多数知识库先用：
  - `EnableHybridSearch=true`
  - `EnableRerank=true`
  - `HybridCandidateMultiplier=4`
  - `HybridRRFK=60`
  - `RerankShortlistMultiplier=2`
- 如果延迟压力更大：
  - 先把 `RerankShortlistMultiplier` 从 `2` 降到 `1`
  - 再视情况把 `HybridCandidateMultiplier` 从 `4` 降到 `2`
- 如果精确词项召回仍不够：
  - 先保持 `HybridRRFK=60`
  - 优先提高 `HybridCandidateMultiplier`

Benchmark 运行方式：

```bash
go test -bench=. -run '^$' ./internal/retrieval ./internal/storage
```

建议至少比较三组：
- vector-only
- hybrid
- hybrid + rerank

## 联网搜索

当前库支持可选的联网搜索能力，首个 provider 是 Tavily。

设计原则：
- 本地知识库优先
- 只有在本地证据不足时才进入联网搜索路径
- 联网搜索通过 `github.com/gtkit/httpc` 完成 HTTP JSON 请求

配置示例：

```go
cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    "https://api.openai.example/v1",
	ChatAPIKey:     "replace-with-your-chat-key",
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey:"replace-with-your-embedding-key",
	EnableWebSearch: true,
	WebSearch: ragagent.WebSearchConfig{
		APIKey:      "tvly-your-key",
		MaxResults:  5,
		SearchDepth: "basic",
		Topic:       "general",
	},
}
```

可选项：
- `BaseURL`
  说明：默认 `https://api.tavily.com/search`
- `MaxResults`
  说明：默认 `5`
- `SearchDepth`
  说明：支持 `basic` / `advanced`，默认 `basic`
- `Topic`
  说明：支持 `general` / `news`，默认 `general`

当前限制：
- 首版只接 Tavily，不支持多 provider 自动切换
- 远程搜索结果当前作为工具文本提供给模型，不进入 `Answer.Citations`
- 本地证据充足时不会主动联网搜索

## 可观测性与降级

当前库除了基础 `Callback` 生命周期回调外，还支持三类可选回调接口：

- `RetrievalMetricsCallback`
  说明：接收一次检索完成后的聚合指标
- `ModelMetricsCallback`
  说明：接收一次模型调用完成后的聚合指标
- `FallbackCallback`
  说明：接收 hybrid / rerank 降级事件

检索指标当前包含：
- `Duration`
- `HybridEnabled`
- `RerankEnabled`
- `VectorCandidateCount`
- `LexicalCandidateCount`
- `FusedCandidateCount`
- `RerankShortlistCount`
- `FinalHitCount`

模型指标当前包含：
- `Model`
- `Duration`
- `Stream`
- `OutputChars`

降级语义：
- hybrid 内部阶段失败时，会退回 `vector_only`
- rerank 阶段失败时，会退回未 rerank 的 `hybrid`
- 基础向量检索本身失败时，不做假降级，直接返回错误

示例：

```go
type metricsObserver struct{}

func (metricsObserver) OnRetrieveStart(context.Context, string)          {}
func (metricsObserver) OnRetrieveEnd(context.Context, int, error)        {}
func (metricsObserver) OnToolStart(context.Context, string)              {}
func (metricsObserver) OnToolEnd(context.Context, string, error)         {}
func (metricsObserver) OnModelStart(context.Context, string)             {}
func (metricsObserver) OnModelEnd(context.Context, string, error)        {}
func (metricsObserver) OnRetrieveMetrics(_ context.Context, m ragagent.RetrievalMetrics) {
	log.Printf("retrieve duration=%s final_hits=%d hybrid=%v rerank=%v",
		m.Duration, m.FinalHitCount, m.HybridEnabled, m.RerankEnabled)
}
func (metricsObserver) OnModelMetrics(_ context.Context, m ragagent.ModelMetrics) {
	log.Printf("model=%s duration=%s stream=%v output_chars=%d",
		m.Model, m.Duration, m.Stream, m.OutputChars)
}
func (metricsObserver) OnFallback(_ context.Context, e ragagent.FallbackEvent) {
	log.Printf("fallback stage=%s to=%s err=%v", e.Stage, e.FallbackTo, e.Err)
}
```

企业级线上建议：
- 先把 `OnRetrieveMetrics`、`OnModelMetrics` 接到你的 metrics / tracing 适配层。
- 对 `OnFallback` 建告警阈值；少量 fallback 可接受，持续升高通常说明参数或数据质量有问题。
- 不要只看总耗时，至少分开看检索耗时和模型耗时。

## 单次执行 Trace

每次问答都会生成一份结构化 `ExecutionTrace`：
- 同步问答：通过 `Answer.Trace` 获取
- 流式问答：在最终 `done` 事件的 `StreamEvent.Trace` 中获取
- 如需在失败路径也保留 trace，可以配置 `Config.TraceRecorder`

trace 当前包含：
- 原始 query 与 rewrite 后 query
- 检索过滤条件
- tool 调用摘要
- `RetrievalMetrics`
- `ModelMetrics`
- fallback 事件
- citation 列表
- 成功 / 失败终态与总耗时

同步示例：

```go
answer, err := agent.GetSession("demo").Ask(ctx, "总结一下架构")
if err != nil {
	log.Fatalf("ask: %v", err)
}
if answer.Trace != nil {
	log.Printf("trace duration=%s final_hits=%d model=%s",
		answer.Trace.Duration,
		answer.Trace.Retrieval.FinalHitCount,
		answer.Trace.Model.Model,
	)
}
```

流式示例：

```go
err := agent.GetSession("demo").AskStream(ctx, "总结一下架构", func(event ragagent.StreamEvent) error {
	if event.Type == ragagent.EventDone && event.Trace != nil {
		log.Printf("stream trace duration=%s tool_calls=%d",
			event.Trace.Duration,
			len(event.Trace.ToolCalls),
		)
	}
	return nil
})
```

如果你希望把 trace 统一送往自定义 sink，可以配置：

```go
type traceSink struct{}

func (traceSink) OnExecutionTrace(_ context.Context, trace ragagent.ExecutionTrace) {
	log.Printf("trace session=%s success=%v", trace.SessionID, trace.Success)
}

cfg := ragagent.Config{
	TraceRecorder: traceSink{},
}
```

如果你希望输出日志摘要，可以配置 `Config.Logger`。库不会自己初始化日志实例；未提供 logger 时保持 no-op。

## 回归评测

仓库内维护了一组不依赖真实外部模型 / embedding 网络调用的 deterministic regression suite，用于锁定：
- runtime 注入
- 同步 trace
- 流式终态 trace
- 联网搜索 fallback 的 tool trace

执行方式：

```bash
go test ./... -run 'TestRuntimeRegressionSuite|TestRuntimeRegressionSuiteDeterministic' -count=1
```

## 内置元数据提取

当前内置 metadata 来源有两类：

1. Markdown YAML front matter
2. sidecar metadata 文件

Markdown 示例：

```md
---
tag: api
lang: zh
project: rag
---

# Gateway API

这里是正文内容。
```

这段 front matter 会被自动提取为 metadata，正文分块里不会再保留这段 YAML。

sidecar metadata 示例：

`gateway.md`

```md
# Gateway API

这里是正文内容。
```

`gateway.meta.json`

```json
{
  "team": "search",
  "lang": "en"
}
```

导入后最终 metadata 会合并为：
- front matter 提供的字段
- sidecar 提供的字段

同名字段以 sidecar 为准。上面这个例子里，如果 `gateway.md` front matter 里也写了 `lang: zh`，最终会以 `gateway.meta.json` 里的 `lang: en` 为准。

限制：
- 当前只支持顶层 metadata 对象。
- metadata 最终会进入 `map[string]string`；复合值会被字符串化。
- `DirSource(path)` 会自动跳过 sidecar 文件本身，不会把它们当正文文档导入。

## 有道笔记桥接导入

当前支持通过本地 `youdaonote` 命令做桥接导入，不直接走有道云笔记 OpenAPI。

用法是创建 `YoudaoNoteSource(...)`，由本地命令先导出到临时目录，再复用现有本地文件导入流程。

示例：

```go
src := ragagent.YoudaoNoteSource(ragagent.YoudaoNoteBridgeConfig{
	Command: "youdaonote",
	Args: []string{
		"export",
		"--output", "{output}",
	},
})

if err := agent.AddKnowledge(ctx, src); err != nil {
	log.Fatalf("add youdao knowledge: %v", err)
}
```

约束：
- 机器上必须已经安装可用的 `youdaonote` 命令。
- `Args` 中至少一个参数必须包含 `{output}` 占位符，运行时会替换成临时导出目录。
- 命令执行失败、命令不存在、或者导出后没有生成受支持文件，都会返回明确错误。

## 会话隔离模型

- `GetSession(id)` 会为同一个 ID 返回稳定复用的 Session。
- 每个 Session 都有自己的历史状态（受 `MaxHistoryRounds` 限制）。
- 同一个 Session 上，`Ask` / `AskStream` 会串行执行。
- 不同 Session 之间可以并发执行。

## 流式行为

`AskStream` 会通过 `StreamEvent` 发送这些事件：
- `retrieve_start`
- `retrieve_end`
- `tool_start`
- `tool_end`
- `answer_chunk`
- `citation`
- `error`
- `done`

如果你的 emitter 回调返回错误，流式过程会立刻停止并把这个错误返回给调用方。

## 检索延迟设计

- 每次查询只做一次 query embedding。
- 向量检索受 `TopK` 和 `SimilarityThreshold` 控制。
- 上下文拼装会按 `ChunkID` 去重，并限制证据总长度（`maxEvidenceChars = 4000`）。
- 检索回调是轻量、同步执行的。

## 生成延迟设计

- 生成阶段优先复用本地检索证据；证据不足且启用联网搜索时，会补充公网搜索结果后再交给聊天模型。
- 每个请求的外部调用受 `RequestTimeout` 控制。
- Session 历史是有界的（`MaxHistoryRounds`），避免 prompt 无限膨胀。
- 流式路径会尽早输出 `answer_chunk`，降低首字节等待感受。

## 自定义工具扩展

Phase 1 的公开 API 还没有开放自定义工具注册能力。

当前的扩展点仍然在内部接线：`agent.go` 里会把 `tools.NewRetrievalTool(...)` 和可选的 `tools.NewWebSearchTool(...)` 传给 `graph.NewChatRunner(...)`。

如果你现在就要加自定义工具，建议在内部 fork / 自定义接线层里扩展，并保持 retrieval tool 的兼容性。
