# go-rag-agent

`go-rag-agent` 是一个面向 Go 服务的可嵌入 RAG / Agent runtime SDK。

它的目标不是提供一套独立平台，而是把本地知识导入、检索增强问答、引用追踪、会话记忆、trace / eval，以及可注入的 runtime / retrieval / storage / memory / tool 边界收敛成一个可以直接嵌入已有 Go 应用的根包 API。

## 项目定位

- `是什么`：库优先的 Go SDK / embeddable runtime
- `是什么`：适合嵌入已有 API 服务、后台任务或内部工具
- `是什么`：支持本地知识导入、会话级记忆、可追溯 citations、trace / eval 和 pgvector 等替换式接线
- `不是什么`：不是开箱即用的 SaaS 控制台
- `不是什么`：不是完整多租户知识平台或独立 control plane
- `不是什么`：不是完整通用 ReAct / orchestration framework

## 适用场景

- `最小嵌入式接入`：先在单机或单服务内把知识导入、问答和 citations 跑通
- `嵌入现有服务`：把 agent 当成 service 依赖注入，按 tenant / source prefix / memory scope 接入现有业务
- `PostgreSQL / pgvector 生产部署`：保留根包 API，不改业务接线，只把向量存储切到 PostgreSQL / pgvector

## 示例与基线导航

- 最小接入：[`examples/basic/main.go`](examples/basic/main.go)
- 服务内嵌：[`examples/service/main.go`](examples/service/main.go)
- pgvector 部署：[`examples/pgvector/main.go`](examples/pgvector/main.go)
- benchmark / eval 基线：[`docs/baselines/sdk-positioning.md`](docs/baselines/sdk-positioning.md)

## API 稳定性分层

- `Stable Core`：`Config` 的基础 provider / retrieval 参数、`New`、`AddKnowledge`、`GetSession`、`Ask` / `AskWithOptions` / `AskStream`、`Answer.Citations`、`RunEvalSuite`、`WriteEvalReportJSON` / `ReadEvalReportJSON`、`NewPGVectorStore` 组成当前主要接入面。除非有显式 migration note，否则不在 `v0.x` 中做静默破坏。
- `Advanced Integration`：`RuntimeComponents`、`RetrievalComponents`、`StorageComponents`、`MemoryComponents`、`ToolRegistry`、`AccessBoundary`、`ProviderGovernance` 属于高级接线层，支持生产使用，但在 `v0.x` 期间仍可能继续补字段和文档。
- `Experimental Assets`：`PDFOCRBridge`、`ImageTextBridge`、`SummarizeExecutionTrace` 的仓库基线用法，以及 `cmd/` 下的辅助生成命令属于实验性资产，优先服务接入和回归，不承诺与 Stable Core 相同的稳定节奏。

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
		ChatModel:        os.Getenv("RAGAGENT_CHAT_MODEL"),
		ChatBaseURL:      os.Getenv("RAGAGENT_CHAT_BASE_URL"),
		ChatAPIKey:       os.Getenv("RAGAGENT_CHAT_API_KEY"),
		EmbeddingModel:   os.Getenv("RAGAGENT_EMBEDDING_MODEL"),
		EmbeddingBaseURL: os.Getenv("RAGAGENT_EMBEDDING_BASE_URL"),
		EmbeddingAPIKey:  os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
		DataDir:          ".rag-data",
		RequestTimeout:   20 * time.Second,
	}
	if cfg.EmbeddingBaseURL == "" {
		cfg.EmbeddingBaseURL = cfg.ChatBaseURL
	}
	if cfg.EmbeddingAPIKey == "" {
		cfg.EmbeddingAPIKey = cfg.ChatAPIKey
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
- `EmbeddingBaseURL` / `EmbeddingAPIKey` 可独立配置；为空时示例会复用 chat provider 的 base URL / API key

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
- `Retrieval`
  说明：可选检索编排注入；支持注入 `Retriever`
- `Storage`
  说明：可选存储/加载/rerank 注入；支持注入 `VectorStore`、`DocumentLoader`、`Reranker`
- `Memory`
  说明：可选长期记忆注入；支持注入 `LongTermMemoryStore`
- `PromptCache`
  说明：可选本地 prompt cache；支持缓存构造后的 prompt messages
- `AccessBoundary`
  说明：可选 namespace 与来源权限边界

示例运行环境变量：

```bash
export RAGAGENT_CHAT_MODEL="gpt-4o-mini"
export RAGAGENT_CHAT_BASE_URL="https://api.example.com/v1"
export RAGAGENT_CHAT_API_KEY="$YOUR_CHAT_API_KEY"
export RAGAGENT_EMBEDDING_MODEL="text-embedding-3-small"
# 如 embedding 与 chat 使用不同服务，再单独设置：
# export RAGAGENT_EMBEDDING_BASE_URL="https://embedding.example.com/v1"
# export RAGAGENT_EMBEDDING_API_KEY="$YOUR_EMBEDDING_API_KEY"

go run ./examples/basic
go run ./examples/service
```

pgvector 示例额外需要 `RAGAGENT_PGVECTOR_DSN`：

```bash
export RAGAGENT_PGVECTOR_DSN="$YOUR_PGVECTOR_DSN"
go run ./examples/pgvector
```

示例代码不会硬编码真实 API key。生产环境请从环境变量、密钥管理服务或运行平台 secret 注入读取凭据，不要把凭据提交到版本库。
- `ToolRegistry`
  说明：可选工具注册表；支持注册或覆写工具
- `ProviderGovernance`
  说明：provider 级错误分类、重试/退避和 usage/cost 聚合配置
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
- `MaxToolCalls` 现在会真实约束工具调用次数：本地检索算一次，后续 fallback 工具链中的每次工具尝试也各算一次。
- `MaxExecutionDuration`
  说明：可选总执行时长预算；超过后会中断整个问答
- `MaxPromptTokens`
  说明：提示词总预算上限，默认 `4096`
- `MaxHistoryTokens`
  说明：历史对话预算上限，默认 `1024`
- `MaxEvidenceTokens`
  说明：证据文本预算上限，默认 `2048`
- `MaxSummaryTokens`
  说明：历史压缩摘要预算上限，默认 `256`
- `MaxMemoryTokens`
  说明：长期记忆回灌预算上限，默认 `512`
- `LongTermMemoryTTL`
  说明：长期记忆 TTL；`<= 0` 表示不过期
- `LongTermMemoryMaxStoredRunes`
  说明：长期记忆写入前的本地压缩上限，默认 `512`
- `EnablePromptHardening`
  说明：是否对检索文本和工具回灌内容做基础 prompt injection 硬化，默认开启
- `LongTermMemoryTopK`
  说明：长期记忆检索条数上限，默认 `3`
- `LongTermMemoryThreshold`
  说明：长期记忆相似度阈值，默认 `0`
- `ProviderGovernance.RetryMaxAttempts`
  说明：provider 调用最大尝试次数，默认 `2`
- `ProviderGovernance.RetryBaseDelay`
  说明：provider 重试基础退避时间，默认 `200ms`
- `ProviderGovernance.RetryMaxDelay`
  说明：provider 重试最大退避时间，默认 `2s`
- `ProviderGovernance.RateLimit.RequestsPerSecond`
  说明：provider 级本地限流速率；`<= 0` 表示关闭，默认关闭
- `ProviderGovernance.RateLimit.Burst`
  说明：provider 级本地限流突发桶大小；启用限流时默认 `1`
- `ProviderGovernance.CircuitBreaker.FailureThreshold`
  说明：provider 级断路器连续失败阈值；`<= 0` 表示关闭，默认关闭
- `ProviderGovernance.CircuitBreaker.OpenTimeout`
  说明：provider 级断路器 open 窗口时长；断路器开启时必须为正数
- `ProviderGovernance.CircuitBreaker.HalfOpenMaxCalls`
  说明：provider 级断路器进入 half-open 后允许的最大探测调用数，默认 `1`
- `PDFOCRBridge` 只有在你要导入扫描版 PDF 时才需要配置；如果配置了，`Args` 必须同时包含 `{input}` 和 `{output}` 占位符。
- `ImageTextBridge` 只有在你要导入图片知识文件时才需要配置；如果配置了，`Args` 必须同时包含 `{input}` 和 `{output}` 占位符。

## 生产验证与部署

生产环境配置从 [`.env.production.example`](.env.production.example) 开始，PostgreSQL / pgvector 可参考 [`deploy/compose/pgvector.compose.yml`](deploy/compose/pgvector.compose.yml)，完整验证矩阵见 [`docs/production.md`](docs/production.md)。

真实 provider 联调默认跳过；只有显式设置 `RAGAGENT_INTEGRATION_LIVE=1` 并通过安全渠道注入 provider 环境变量后，才运行 `TestLiveOpenAICompatibleProviders`。

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
	BaseURL: os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	APIKey:  os.Getenv("RAGAGENT_CHAT_API_KEY"),
	Timeout: 20 * time.Second,
})
if err != nil {
	log.Fatalf("new chat model: %v", err)
}

embedder, err := ragagent.NewOpenAIEmbedder(ctx, ragagent.EmbedderConfig{
	Model:   "text-embedding-3-small",
	BaseURL: os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	APIKey:  os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
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
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	Storage: ragagent.StorageComponents{
		VectorStore:    store,
		DocumentLoader: ragagent.NewFileDocumentLoader(),
		Reranker:       ragagent.NewRuleBasedReranker(),
	},
}
```

如果你有自己的向量库、文档加载器或重排器，只要实现根包公开的接口即可。默认行为不变，只有你显式注入的部分会被覆盖。

## 自定义 Retriever

如果你希望替换整个检索编排，而不只是替换底层 `VectorStore`，现在也可以通过 `Config.Retrieval` 注入根包公开的 `Retriever`。

默认构造器：
- `NewRetriever(store, embedder, reranker, cfg)`

这条边界适合接管：
- 查询到检索请求的转换
- source path / metadata filter 的解释
- hybrid / rerank 的执行策略

示例：

```go
retriever := ragagent.NewRetriever(
	store,
	embedder,
	ragagent.NewRuleBasedReranker(),
	ragagent.RetrieverConfig{
		TopK:                    5,
		SimilarityThreshold:     0,
		EnableHybridSearch:      true,
		EnableRerank:            true,
		HybridCandidateMultiply: 4,
		HybridRRFK:              60,
		RerankShortlistMultiple: 2,
	},
)

cfg := ragagent.Config{
	Runtime: ragagent.RuntimeComponents{
		ChatModel: chatModel,
		Embedder:  embedder,
	},
	Retrieval: ragagent.RetrievalComponents{
		Retriever: retriever,
	},
}
```

## Prompt Cache

当前库支持本地 prompt messages cache，用于复用相同 deterministic request 的 prompt 构造结果。

默认实现：
- `NewInMemoryPromptCache()`
- `NewInMemoryPromptCacheWithConfig(cfg)`
- `NewRedisPromptCache(client, cfg)`
- `NewTwoLevelPromptCache(local, remote)`

示例：

```go
cfg := ragagent.Config{
	PromptCache: ragagent.NewInMemoryPromptCache(),
}
```

自定义边界：

```go
cache := ragagent.NewInMemoryPromptCacheWithConfig(ragagent.PromptCacheConfig{
	MaxEntries: 512,
	TTL:        10 * time.Minute,
})
```

当前语义：
- 命中 cache 时会跳过重复的 prompt messages 构造
- `ExecutionTrace.PromptCacheHit` 会标记本次是否命中本地 cache
- 这是本地 prompt artifact cache，不是 provider-native prompt cache
- 默认内存实现已经是有界缓存，具备 `LRU + TTL + max entries`

维护接口：
- 内存 prompt cache 还支持 `PromptCacheMaintenance`
- `Delete(ctx, keys...) error`
- `Clear(ctx) error`
- `Stats(ctx) (PromptCacheStats, error)`

Redis 两级缓存示例：

```go
local := ragagent.NewInMemoryPromptCacheWithConfig(ragagent.PromptCacheConfig{
	MaxEntries: 512,
	TTL:        10 * time.Minute,
})

remote := ragagent.NewRedisPromptCache(redisClient, ragagent.RedisPromptCacheConfig{
	KeyPrefix: "ragagent:prompt",
	TTL:       30 * time.Minute,
})

cfg := ragagent.Config{
	PromptCache: ragagent.NewTwoLevelPromptCache(local, remote),
}
```

## Access Boundary

当前库支持静态 namespace 与来源权限边界：

```go
cfg := ragagent.Config{
	AccessBoundary: ragagent.AccessBoundaryConfig{
		Namespace:             "tenant-a",
		AllowedSourcePaths:    []string{"/kb/tenant-a/doc.md"},
		AllowedSourcePrefixes: []string{"/kb/tenant-a/"},
	},
}
```

当前语义：
- 导入知识时会把 namespace 写入 chunk metadata
- 检索时会自动把 namespace 与 allowed source 边界合并进 filter
- query 不能越过配置允许的来源范围

动态 policy hook 示例：

```go
type myPolicy struct{}

func (myPolicy) TransformKnowledgeFile(_ context.Context, file ragagent.KnowledgeFile) (ragagent.KnowledgeFile, error) {
	if file.Metadata == nil {
		file.Metadata = map[string]string{}
	}
	file.Metadata["team"] = "alpha"
	return file, nil
}

func (myPolicy) ConstrainRetrieval(_ context.Context, req ragagent.AccessPolicyRequest) (ragagent.RetrievalFilter, error) {
	return ragagent.RetrievalFilter{
		SourcePrefixes: []string{"/kb/alpha/"},
	}, nil
}

cfg := ragagent.Config{
	AccessBoundary: ragagent.AccessBoundaryConfig{
		Namespace: "tenant-a",
		Policy:    myPolicy{},
	},
}
```

当前语义补充：
- 动态 policy 可以在导入时增强 `KnowledgeFile`
- 动态 policy 可以在检索时收敛 `RetrievalFilter`
- 静态 `Namespace` / `AllowedSourcePaths` / `AllowedSourcePrefixes` 仍然是最终上界

## Eval Runner

当前库支持根包 deterministic eval runner：

```go
summary, results, err := ragagent.RunEvalSuite(ctx, agent, []ragagent.EvalCase{
	{
		Name:                   "basic",
		SessionID:              "eval",
		Query:                  "what changed",
		WantCitationSources:    []string{"/tmp/doc.md"},
		WantAnswerContains:     []string{"ok"},
		WantGroundedSubstrings: []string{"ok"},
	},
})
_ = summary
_ = results
_ = err
```

结构化输出评测：

```go
results, err := ragagent.RunStructuredEvalSuite(ctx, agent, []ragagent.StructuredEvalCase{
	{
		Name:      "structured",
		SessionID: "eval-structured",
		Query:     "status?",
		NewTarget: func() any { return &MyTarget{} },
		Validate: func(target any) error {
			return nil
		},
	},
})
_ = results
_ = err
```

## 长期记忆分层

当前库现在区分两层记忆：
- 短期记忆
  当前 Session 内的最近对话历史，受 `MaxHistoryRounds` 和 token 预算控制
- 长期记忆
  可选的同 Session 语义记忆层，在短期窗口外仍可被检索并回灌 prompt

默认长期记忆实现：
- `NewInMemoryLongTermMemoryStore()`
- `NewVectorLongTermMemoryStore(store)`

示例：

```go
memoryStore := ragagent.NewInMemoryLongTermMemoryStore()

cfg := ragagent.Config{
	ChatModel:       "gpt-4o-mini",
	ChatBaseURL:     os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:      os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel:  "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	Memory: ragagent.MemoryComponents{
		LongTermMemory: memoryStore,
	},
	LongTermMemoryTopK:      3,
	LongTermMemoryThreshold: 0,
	MaxMemoryTokens:         512,
}
```

当前语义：
- 每次同步/流式问答成功后，会把该轮 `query + answer` 写入长期记忆
- 后续问答前，会按当前 query 检索同 Session 的长期记忆
- 命中的长期记忆会以 `Relevant long-term memory` 独立块注入 prompt
- 如果你把长期记忆接到持久化 `VectorStore`，长期记忆也会跨进程保留
- 相同 session/user/tenant 下相同内容的长期记忆会做确定性去重
- `LongTermMemoryTTL` 配置后，过期记忆不会再参与检索
- 超长 query/answer 会按 `LongTermMemoryMaxStoredRunes` 做本地确定性压缩
- 可以通过 `QueryOptions.MemoryScope` 继续按 user/tenant 分层；tenant 为空时回退到 `AccessBoundary.Namespace`

如果你已有外部记忆服务，可以通过 `MemoryProvider` 接入问答生命周期：
- `Retrieve`
  说明：问答前返回可注入的 memory text / messages
- `Memorize`
  说明：问答成功后写入本轮 user query 与 assistant answer

现有长期记忆 store 也可以适配为 provider：

```go
store := ragagent.NewInMemoryLongTermMemoryStore()
provider := ragagent.NewLongTermMemoryProvider(store, embedder, ragagent.LongTermMemoryProviderConfig{
	TopK:      3,
	Threshold: 0,
})

cfg := ragagent.Config{
	Memory: ragagent.MemoryComponents{
		Provider:              provider,
		ProviderFailurePolicy: ragagent.MemoryFailurePolicyFailOpen,
	},
}
```

`MemoryFailurePolicyFailClosed` 会在记忆检索失败时返回错误；`MemoryFailurePolicyFailOpen` 会记录 trace 并按空记忆降级。

当前限制：
- 第一版只做同 Session 长期记忆，不做跨 Session 共享
- 默认实现仍然是进程内存；持久化需要显式注入 `NewVectorLongTermMemoryStore(...)`

相关配置：
- `LongTermMemoryTTL`
  说明：长期记忆 TTL；`<= 0` 表示不过期
- `LongTermMemoryMaxStoredRunes`
  说明：长期记忆写入前的本地压缩上限，默认 `512`

按 user/tenant 分层示例：

```go
answer, err := session.AskWithOptions(ctx, "follow up", ragagent.QueryOptions{
	MemoryScope: ragagent.MemoryScope{
		UserID: "user-a",
		Tenant: "tenant-a",
	},
})
_ = answer
_ = err
```

维护接口：
- in-memory 长期记忆支持 `LongTermMemoryMaintenance`
- 可显式执行过期清理

统一维护入口：

```go
if err := agent.Maintain(ctx, time.Now()); err != nil {
	panic(err)
}
```

示例：复用持久化 `pgvector` 作为长期记忆后端

```go
memoryVectorStore, err := ragagent.NewPGVectorStore(ragagent.PGVectorStoreConfig{
	ConnString: os.Getenv("RAGAGENT_PGVECTOR_DSN"),
	TableName:  "long_term_memories",
	Dimensions: 1536,
})
if err != nil {
	log.Fatalf("new long-term memory store: %v", err)
}

cfg := ragagent.Config{
	ChatModel:       "gpt-4o-mini",
	ChatBaseURL:     os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:      os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel:  "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	Memory: ragagent.MemoryComponents{
		LongTermMemory: ragagent.NewVectorLongTermMemoryStore(memoryVectorStore),
	},
}
```

## Embedded Mode 与 Server Mode

当前库支持两种向量存储模式：

- `embedded mode`
  默认模式，使用 `chromem-go`
  适合单机、本地优先、零外部数据库依赖
- `server mode`
  可选模式，使用 `PostgreSQL/pgvector`
适合多实例共享知识库、持久化备份、数据库运维和服务端部署

如果你不注入 `Config.Storage.VectorStore`，库会继续使用默认的 embedded mode。

### PostgreSQL / pgvector

你可以通过 `NewPGVectorStore(...)` 把 PostgreSQL/pgvector 注入到现有 `Agent` 主流程中：

```go
store, err := ragagent.NewPGVectorStore(ragagent.PGVectorStoreConfig{
	ConnString: os.Getenv("RAGAGENT_PGVECTOR_DSN"),
	TableName:  "knowledge_chunks",
	Dimensions: 1536,
})
if err != nil {
	log.Fatalf("new pgvector store: %v", err)
}

cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	Storage: ragagent.StorageComponents{
		VectorStore:    store,
		DocumentLoader: ragagent.NewFileDocumentLoader(),
		Reranker:       ragagent.NewRuleBasedReranker(),
	},
}
```

如果你的 PostgreSQL 连接层已经统一使用 `pgorm`，`PGVectorStore` 现在也支持两种接法：

```go
pgCfg := pgorm.NewConfig(
	pgorm.WithDSN(os.Getenv("RAGAGENT_PGVECTOR_DSN")),
	pgorm.WithStartupPing(false),
)

store, err := ragagent.NewPGVectorStore(ragagent.PGVectorStoreConfig{
	PGORMConfig: &pgCfg,
	TableName:   "knowledge_chunks",
	Dimensions:  1536,
})
if err != nil {
	log.Fatalf("new pgvector store with pgorm config: %v", err)
}
```

或者复用一个外部已经打开的 `pgorm.Client`：

```go
client, err := pgorm.Open(ctx,
	pgorm.WithDSN(os.Getenv("RAGAGENT_PGVECTOR_DSN")),
	pgorm.WithStartupPing(false),
)
if err != nil {
	log.Fatalf("open pgorm client: %v", err)
}
defer client.Close()

store, err := ragagent.NewPGVectorStore(ragagent.PGVectorStoreConfig{
	PGORMClient: client,
	TableName:   "knowledge_chunks",
	Dimensions:  1536,
})
if err != nil {
	log.Fatalf("new pgvector store with pgorm client: %v", err)
}
```

当前第一版约束：
- `Dimensions` 必填
- 第一版只正式支持 `DistanceMetric="cosine"`
- 默认索引策略是 `none`，即 exact search
- `HNSW` / `IVFFlat` 是可选后续索引策略，不会默认启用
- `UpsertBatchSize` 控制单次写入 batch 大小，默认 `200`
- `ConnString`、`Pool`、`PGORMConfig`、`PGORMClient` 四种连接来源必须且只能提供一种

生产建议：
- knowledge 与 long-term memory 最好分表，而不是共用同一张 `pgvector` 表
- 默认先从 `IndexStrategy=none` 起步，数据量上来后再切 `HNSW`
- 如果是高频导入场景，优先调 `UpsertBatchSize` 再考虑更重的导入机制

集成测试说明：
- PostgreSQL/pgvector integration tests 通过环境变量 `RAGAGENT_PGVECTOR_TEST_DSN` 启用
- 未设置该环境变量时，相关 integration tests 会自动跳过

## 知识导入流程

1. 通过 `FileSource(path)` 或 `DirSource(path)` 提供来源。
   当前支持：
   - `.txt`
   - `.md`
   - `.html` / `.htm`
   - 文本型 `.pdf`
   - 扫描版 `.pdf`（配置 OCR bridge 后）
   - `.png` / `.jpg` / `.jpeg` / `.webp`（配置图片 bridge 后）
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
- HTML 会抽取可见文本，并跳过 `script` / `style` / `noscript` 等非正文内容。
- 图片文件会通过 `ImageTextBridge` 转成纯文本后导入；未配置 bridge 时会返回明确错误。
- Markdown 支持 YAML front matter，导入时会自动提取为 metadata，并从正文中剥离该 front matter。
- 所有支持的知识文件都支持 sidecar metadata，命名规则是 `<basename>.meta.json|yaml|yml`。
- 如果同一个 Markdown 同时存在 front matter 和 sidecar metadata，sidecar 的同名字段会覆盖 front matter。

知识生命周期 API：
- `AddKnowledge(ctx, src)`
  说明：导入或更新一个知识源
- `RemoveKnowledge(ctx, src)`
  说明：显式删除一个知识源已导入的内容
- `RebuildKnowledge(ctx, src)`
  说明：先删除该知识源已有内容，再重新导入当前内容

示例：

```go
if err := agent.RemoveKnowledge(ctx, ragagent.FileSource("/tmp/old.md")); err != nil {
	log.Fatalf("remove knowledge: %v", err)
}

if err := agent.RebuildKnowledge(ctx, ragagent.DirSource("/tmp/knowledge")); err != nil {
	log.Fatalf("rebuild knowledge: %v", err)
}
```

## 文档转换器扩展

默认 `FileSource(path)` 仍然只接受内置支持的文本、HTML、PDF 和图片扩展名。对于 `.docx`、复杂 PDF 预处理、Office 文档或自定义网页清洗，可以显式使用 `ConvertibleFileSource(path)` 并在 `Config.DocumentConverters` 中注入转换器。

命令型转换器示例：

```go
converter, err := ragagent.NewCommandDocumentConverter(ragagent.CommandDocumentConverterConfig{
    Name:       "markitdown",
    Extensions: []string{".docx", ".pptx", ".xlsx"},
    Command:    "markitdown",
    Args:       []string{"{input}", "--output", "{output}"},
})
if err != nil {
    log.Fatalf("new converter: %v", err)
}

cfg := ragagent.Config{
    ChatModel:          "gpt-4o-mini",
    ChatBaseURL:        os.Getenv("RAGAGENT_CHAT_BASE_URL"),
    ChatAPIKey:         os.Getenv("RAGAGENT_CHAT_API_KEY"),
    EmbeddingModel:     "text-embedding-3-small",
    EmbeddingAPIKey:    os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
    DocumentConverters: []ragagent.DocumentConverter{converter},
}

if err := agent.AddKnowledge(ctx, ragagent.ConvertibleFileSource("knowledge/handbook.docx")); err != nil {
    log.Fatalf("add converted knowledge: %v", err)
}
```

约束：
- `FileSource(path)` 的严格扩展名校验不变；需要转换器时使用 `ConvertibleFileSource(path)`。
- 命令参数必须包含 `{input}` 和 `{output}` 占位符。
- 转换器输出必须是纯文本或 Markdown；空输出会返回错误，不会静默导入空知识。
- `DirSource(path)` 仍然只扫描内置支持的扩展名；目录级 Office 扫描建议由调用方先生成清单后逐个使用 `ConvertibleFileSource`。

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
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
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

## 图片导入 Bridge

如果你的知识库里包含图片文件，可以在 `Config` 里配置 `ImageTextBridge`。

设计约束是：
- 库本身不绑定某个 OCR 或 vision SDK
- 你提供本地 bridge 命令
- 命令读取 `{input}` 指向的图片，并把输出文本写入 `{output}` 指向的文本文件

示例：

```go
cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	ImageTextBridge: ragagent.ImageTextBridgeConfig{
		Command: "my-image-text",
		Args: []string{
			"{input}",
			"{output}",
		},
	},
}
```

当前支持的图片扩展名：
- `.png`
- `.jpg`
- `.jpeg`
- `.webp`

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
	ChatBaseURL:       os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:        os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel:    "text-embedding-3-small",
	EmbeddingAPIKey:   os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
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
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
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
- 远程搜索结果在默认 `search_web` 路径下会进入 `Answer.Citations`
- 对这些远端 citations，`Citation.SourcePath` 表示 canonical URL
- 本地证据充足时不会主动联网搜索

## 工具注册表

当前库已经开放根包 `Tool` 与 `ToolRegistry`。

默认工具：
- `retrieve_context`
  说明：本地检索工具，默认始终存在
- `search_web`
  说明：当 `EnableWebSearch=true` 且调用方未覆写时，默认注册 Tavily 搜索工具

你可以通过 `Config.ToolRegistry` 注册自定义 fallback 工具，或用同名工具覆写默认实现。

示例：

```go
type myTool struct{}

func (myTool) Name() string { return "search_internal" }
func (myTool) Description() string { return "Search internal services" }
func (myTool) Run(ctx context.Context, input string) (string, error) {
	return "internal result", nil
}

registry := ragagent.NewToolRegistry()
if err := registry.Register(myTool{}); err != nil {
	log.Fatalf("register tool: %v", err)
}

cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	ToolRegistry:   registry,
}
```

MCP HTTP JSON-RPC 工具适配示例：

示例中的 HTTP client 使用 `github.com/gtkit/httpc`。

```go
mcpTool, err := ragagent.NewMCPTool(httpc.New(httpc.WithTimeout(10*time.Second)), ragagent.MCPToolConfig{
    Name:        "search_docs",
    Description: "Search remote docs through MCP",
    Endpoint:    "https://mcp.example/rpc",
    Method:      "tools/call",
    ToolName:    "search_docs",
})
if err != nil {
    log.Fatalf("new mcp tool: %v", err)
}

registry := ragagent.NewToolRegistry(mcpTool)
```

当前 MCP 适配器只覆盖 HTTP JSON-RPC `tools/call` 形态；不做 stdio MCP session 管理、工具自动发现或完整 ReAct loop。

工具执行语义：
- 本地检索始终优先
- 只有在本地证据不足时，才会进入 fallback 工具链
- fallback 工具按注册顺序尝试
- 如果调用方注册同名 `search_web`，会覆写默认 web 工具
- 默认工具链主要用于 evidence-empty fallback

## 结构化工具与 Tool Calling

除了基础 `Tool` 文本接口，当前库还提供结构化工具契约：
- `StructuredTool`
  说明：声明 `ToolSchema`，接收结构化 JSON 参数，返回 `ToolResult`
- `NewStructuredToolAdapter(tool)`
  说明：把结构化工具适配成现有 `Tool`，可直接注册到 `ToolRegistry`
- `ValidateToolArguments(schema, args)`
  说明：在工具业务逻辑前执行 required、类型、enum、字符串长度和数值边界校验

```go
type lookupTool struct{}

func (lookupTool) Name() string { return "lookup" }
func (lookupTool) Description() string { return "Look up safe internal data" }
func (lookupTool) Schema() ragagent.ToolSchema {
	return ragagent.ToolSchema{
		Properties: map[string]ragagent.ToolParameterSchema{
			"query": {Type: ragagent.ToolParameterString, MinLength: 1, MaxLength: 128},
		},
		Required: []string{"query"},
	}
}
func (lookupTool) RunStructured(ctx context.Context, args map[string]any) (ragagent.ToolResult, error) {
	return ragagent.ToolResult{Text: "result for " + args["query"].(string)}, nil
}

registry := ragagent.NewToolRegistry(ragagent.NewStructuredToolAdapter(lookupTool{}))
```

Tool calling 默认关闭。显式启用后，SDK 会使用有限的单 agent 工具循环：

```go
cfg := ragagent.Config{
	EnableToolCalling: true,
	ToolRegistry:      registry,
	MaxToolCalls:      4,
	MaxIterations:     3,
}
```

生产安全边界：
- SDK 不内置 Shell、数据库执行、文件写入或任务调度工具。
- 所有可调用工具都必须由宿主服务显式注册。
- 启用 tool calling 时必须配置 `ToolRegistry`，否则配置校验失败。
- 工具循环同时受 `MaxToolCalls`、`MaxIterations` 和调用方 `context.Context` 控制。
- 工具实现应由宿主服务自行做鉴权、租户隔离、输入长度限制、超时、审计和脱敏。

## 外部 Reranker 适配

`Reranker` 仍然是检索重排的公开接口。除了默认规则重排器，你也可以注入 OpenAI-compatible HTTP rerank endpoint：

示例中的 HTTP client 使用 `github.com/gtkit/httpc`。

```go
reranker, err := ragagent.NewOpenAIReranker(
    httpc.New(httpc.WithTimeout(10*time.Second)),
    ragagent.OpenAIRerankerConfig{
        BaseURL: "https://rerank.example/v1/rerank",
        APIKey:  os.Getenv("RAGAGENT_RERANK_API_KEY"),
        Model:   "rerank-model",
    },
)
if err != nil {
    log.Fatalf("new reranker: %v", err)
}

cfg := ragagent.Config{
    EnableHybridSearch: true,
    EnableRerank:       true,
    Storage: ragagent.StorageComponents{
        Reranker: reranker,
    },
}
```

语义：
- 只把 hybrid shortlist 发送给外部 reranker。
- HTTP 错误、非法 index 或无效响应会返回给现有 rerank 降级路径，主流程可退回未 rerank 的 hybrid 结果。
- SDK 不内置具体商业平台模型常量；平台差异可通过 `Reranker` 接口自行适配。

## Gateway 示例

仓库提供 `examples/gateway`，演示如何在已有 `net/http` 服务里暴露最小非流式 OpenAI-compatible `/v1/chat/completions` handler，并把请求转给 `Agent.GetSession(...).Ask(...)`。

边界：
- 这是嵌入示例，不是内置控制台。
- 不包含用户系统、后台管理、知识库 CRUD 或 SaaS control plane。
- 流式 chat completions、鉴权、限流和租户权限应由宿主服务按自身架构实现。

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

如果你希望输出更稳定的聚合字段，而不是自己解析完整 trace，可以使用：

```go
summary := ragagent.SummarizeExecutionTrace(*answer.Trace)
log.Printf("trace summary session=%s success=%v tools=%d citations=%d",
	summary.SessionID,
	summary.Success,
	summary.ToolCallCount,
	summary.CitationCount,
)
```

标准 recorder adapter：
- `NewMultiTraceRecorder(...)`
  说明：fan-out 到多个 recorder，并隔离单个 recorder 的 panic
- `NewJSONLTraceRecorder(writer)`
  说明：输出一行安全摘要 JSON，不输出完整 prompt、evidence 或密钥
- `NewLoggerTraceRecorder(logger)`
  说明：复用现有 `Logger` 接口输出执行摘要和 fallback 告警

示例：

```go
var traceLog bytes.Buffer
cfg := ragagent.Config{
	TraceRecorder: ragagent.NewMultiTraceRecorder(
		ragagent.NewJSONLTraceRecorder(&traceLog),
		ragagent.NewLoggerTraceRecorder(logger),
	),
}
```

如果你希望输出日志摘要，可以配置 `Config.Logger`。库不会自己初始化日志实例；未提供 logger 时保持 no-op。

## Provider 治理与 usage/cost 聚合

当前库已经在 provider 层增加五件事：
- 错误分类
- 有限次重试与退避
- 可选的本地限流
- 可选的断路器
- usage/cost 聚合

覆盖范围：
- OpenAI-compatible chat
- OpenAI-compatible embedding
- Tavily web search

示例：

```go
cfg := ragagent.Config{
	ChatModel:      "gpt-4o-mini",
	ChatBaseURL:    os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:     os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel: "text-embedding-3-small",
	EmbeddingAPIKey: os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	ProviderGovernance: ragagent.ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   200 * time.Millisecond,
		RetryMaxDelay:    2 * time.Second,
		RateLimit: ragagent.ProviderRateLimitConfig{
			RequestsPerSecond: 5,
			Burst:             2,
		},
		CircuitBreaker: ragagent.ProviderCircuitBreakerConfig{
			FailureThreshold: 3,
			OpenTimeout:      30 * time.Second,
			HalfOpenMaxCalls: 1,
		},
		Pricing: ragagent.ProviderPricingConfig{
			ChatModels: map[string]ragagent.TokenPricing{
				"gpt-4o-mini": {
					InputUSDPer1K:  0.01,
					OutputUSDPer1K: 0.02,
				},
			},
			EmbeddingModels: map[string]ragagent.TokenPricing{
				"text-embedding-3-small": {
					InputUSDPer1K: 0.0001,
				},
			},
			WebSearchPerCallUSD: 0,
		},
	},
}
```

当前语义：
- `429`、瞬时网络错误、部分 `5xx` 会被识别为可重试错误
- stream 只有在尚未输出任何 chunk 时才允许自动重试
- 同一 provider 名下的聊天、embedding、web search 会共享本地治理状态
- 本地限流只在显式配置 `RateLimit.RequestsPerSecond > 0` 时生效
- 断路器只在显式配置 `CircuitBreaker.FailureThreshold > 0` 且 `OpenTimeout > 0` 时生效
- 认证错误和 permanent 错误不会打开断路器
- provider usage / estimated cost 会聚合到 `ExecutionTrace.ProviderCalls`
- provider trace 还会记录 `ThrottleDelay` 和 `CircuitState`

## 上下文窗口治理与提示硬化

当前库已经在 prompt 构造阶段增加基础治理能力：
- 历史对话按 token 预算裁剪
- 被裁掉的旧历史会做本地摘要压缩
- 检索证据按 token 预算裁剪
- 检索文本和工具回灌文本会经过基础 prompt injection 硬化
- 可以为整个问答流程设置总执行时长预算

示例：

```go
cfg := ragagent.Config{
	ChatModel:           "gpt-4o-mini",
	ChatBaseURL:         os.Getenv("RAGAGENT_CHAT_BASE_URL"),
	ChatAPIKey:          os.Getenv("RAGAGENT_CHAT_API_KEY"),
	EmbeddingModel:      "text-embedding-3-small",
	EmbeddingAPIKey:     os.Getenv("RAGAGENT_EMBEDDING_API_KEY"),
	MaxPromptTokens:     4096,
	MaxHistoryTokens:    1024,
	MaxEvidenceTokens:   2048,
	MaxSummaryTokens:    256,
	MaxExecutionDuration: 15 * time.Second,
	EnablePromptHardening: true,
}
```

当前语义：
- 历史窗口优先保留最近轮次
- 超出预算的旧历史会压缩成 `Conversation summary`
- 检索证据和 fallback 工具结果会被视为不可信文本
- 命中明显 prompt injection 模式的行会被替换为 `[filtered potential prompt injection]`

当前限制：
- 这是轻量级本地 hardening，不是完整安全沙箱
- 还没有做模型级 prompt cache

## 结构化输出

当前库支持同步结构化输出：
- `AskStructured(ctx, query, target)`
- `AskStructuredWithOptions(ctx, query, opts, target)`

使用方式是传入一个非 nil 指针目标，库会：
1. 继续走现有 RAG 检索链
2. 要求模型只返回 JSON
3. 尝试从模型文本中提取 JSON
4. 把 JSON 反序列化到你提供的目标结构

示例：

```go
type Summary struct {
	Summary string `json:"summary"`
	Score   int    `json:"score"`
}

var out Summary
result, err := agent.GetSession("demo").AskStructured(ctx, "总结一下文档", &out)
if err != nil {
	log.Fatalf("ask structured: %v", err)
}

fmt.Println(out.Summary)
fmt.Println(result.RawJSON)
fmt.Println(result.Answer.Citations)
```

当前约束：
- `target` 必须是非 nil 指针
- 当前只支持同步结构化输出，不支持流式结构化输出
- 第一版不接模型原生 function-calling / JSON schema 协议
- 模型如果返回 fenced code block 或前后带说明文字，库会尝试提取其中的首个有效 JSON

## 回归评测

仓库内维护了一组不依赖真实外部模型 / embedding 网络调用的 deterministic regression suite，用于锁定：
- runtime 注入
- 同步 trace
- 流式终态 trace
- 联网搜索 fallback 的 tool trace
- 联网搜索 citations

你也可以把 `EvalReport` 直接落盘，作为后续基线比较输入：

```go
report := ragagent.EvalReport{
	Summary: summary,
	Results: results,
}
if err := ragagent.WriteEvalReportJSON("eval-report.json", report); err != nil {
	log.Fatalf("write eval report: %v", err)
}

baseline, err := ragagent.ReadEvalReportJSON("eval-report.json")
if err != nil {
	log.Fatalf("read eval report: %v", err)
}
if err := ragagent.CompareEvalReportWithBaseline(report, baseline); err != nil {
	log.Fatalf("baseline compare failed: %v", err)
}
```

仓库内也维护了一份可提交、可复现的 SDK 定位基线资产：

- 说明文档：[`docs/baselines/sdk-positioning.md`](docs/baselines/sdk-positioning.md)
- 默认生成命令：`go run ./cmd/generate-sdk-positioning-baseline`

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

当前库已经通过 `Config.ToolRegistry` 开放自定义工具注册能力。

现阶段限制：
- 当前工具注册表主要服务于 evidence-empty fallback 工具链
- 不是完整的模型驱动任意工具调用协议
- `retrieve_context` 仍然由主流程优先执行，不由模型自由选择
