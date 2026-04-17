# go-rag-agent

`go-rag-agent` 是一个基于 Go 1.26.2 的本地优先 RAG 运行时，提供：
- 本地知识导入
- chromem 向量检索
- 会话记忆
- OpenAI-compatible 聊天 / embedding 适配

## 安装

当前模块路径是 `my-gtkit-package/go-rag-agent`，它在这个仓库里按本地/内部模块路径使用。

推荐在本地工作区或内部代码仓中引用它，而不是按公网模块直接 `go get`。

示例：

```go
require my-gtkit-package/go-rag-agent v0.0.0

replace my-gtkit-package/go-rag-agent => ../go-rag-agent
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

	ragagent "my-gtkit-package/go-rag-agent"
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
- `ChatModel`
- `ChatBaseURL`
- `ChatAPIKey`
- `EmbeddingModel`

可选项（由 `New` / `Validate` 应用默认值）：
- `TopK`（默认 `5`）
- `ChunkSize`（默认 `1000`）
- `MaxHistoryRounds`（默认 `8`）
- `MaxIterations`（默认 `3`）
- `RequestTimeout`（默认 `30s`）

校验说明：
- 空 `DataDir` 表示使用内存模式，不会强制写入当前目录。
- `SimilarityThreshold: 0` 会保留非负相似度结果；如果你希望连负相似度结果也保留，需要传负值。
- `ChunkSize` 必须不超过当前证据拼装预算（`<= 4000` rune）。
- `ChunkOverlap` 必须满足 `>= 0` 且 `< ChunkSize`。
- `EnableHybridSearch` 和 `EnableRerank` 在 Phase 1 会被拒绝。
- `MaxToolCalls` 目前在 Phase 1 里保留字段，但还没有真正接入运行时控制。
- `PDFOCRBridge` 只有在你要导入扫描版 PDF 时才需要配置；如果配置了，`Args` 必须同时包含 `{input}` 和 `{output}` 占位符。

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

- 模型 / 工具循环受 `MaxIterations` 约束。
- 每个请求的外部调用受 `RequestTimeout` 控制。
- Session 历史是有界的（`MaxHistoryRounds`），避免 prompt 无限膨胀。
- 流式路径会尽早输出 `answer_chunk`，降低首字节等待感受。

## 自定义工具扩展

Phase 1 的公开 API 还没有开放自定义工具注册能力。

当前的扩展点仍然在内部接线：`agent.go` 里会把 `tools.NewRetrievalTool(...)` 传给 `graph.NewReactRunner(...)`。

如果你现在就要加自定义工具，建议在内部 fork / 自定义接线层里扩展，并保持 retrieval tool 的兼容性。
