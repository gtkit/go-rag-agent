# go-rag-agent

`go-rag-agent` is a Go 1.26.2 RAG runtime with local ingestion, chromem-backed vector retrieval, session memory, and OpenAI-compatible chat/embedding adapters.

## Installation

```bash
go get my-gtkit-package/go-rag-agent
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
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

	if err := agent.AddKnowledge(ctx, ragagent.DirSource("./knowledge")); err != nil {
		log.Fatalf("add knowledge: %v", err)
	}

	answer, err := agent.GetSession("demo").Ask(ctx, "Summarize the architecture in knowledge/")
	if err != nil {
		log.Fatalf("ask: %v", err)
	}
	fmt.Println(answer.Text)
}
```

## Config Reference

Required:
- `ChatModel`
- `ChatBaseURL`
- `ChatAPIKey`
- `EmbeddingModel`

Optional with defaults (`Config.withDefaults`):
- `DataDir` (`"."`)
- `TopK` (`5`)
- `SimilarityThreshold` (`0.6`)
- `ChunkSize` (`1000`)
- `MaxHistoryRounds` (`8`)
- `MaxToolCalls` (`4`)
- `MaxIterations` (`3`)
- `RequestTimeout` (`30s`)

Validation notes:
- `ChunkOverlap` must be `>=0` and `< ChunkSize`.
- `EnableHybridSearch` and `EnableRerank` are rejected in phase 1.

## Ingestion Workflow

1. Provide a source via `FileSource(path)` or `DirSource(path)` (`.txt` and `.md` only).
2. `AddKnowledge` resolves files and loads content into RAG documents.
3. Content is chunked by rune window (`ChunkSize`, `ChunkOverlap`).
4. Chunks are embedded through the configured embedding adapter.
5. Records are upserted into local chromem storage.

## Session Isolation Model

- `GetSession(id)` returns one stable session instance per ID.
- Each session has independent history state (`MaxHistoryRounds` bounded).
- Same-session asks are serialized (`Ask`/`AskStream` cannot execute concurrently on the same session).
- Different sessions can run concurrently.

## Streaming Behavior

`AskStream` emits `StreamEvent` values with these types:
- `retrieve_start`, `retrieve_end`
- `tool_start`, `tool_end`
- `answer_chunk`
- `citation`
- `error`
- `done`

If your emitter callback returns an error, streaming stops and the call returns that error.

## Retrieval Latency Design

- Single-query embedding per ask.
- Vector search is bounded by `TopK` and `SimilarityThreshold`.
- Context assembly deduplicates chunk IDs and caps evidence size (`maxEvidenceChars = 4000`).
- Retrieval callback hooks are lightweight and synchronous.

## Generation Latency Design

- Model/tool loop is bounded by `MaxIterations`.
- Per-request external calls use `RequestTimeout`.
- Session history is bounded (`MaxHistoryRounds`) to avoid unbounded prompt growth.
- Streaming path emits incremental answer chunks to reduce time-to-first-token perception.

## Custom Tool Integration Approach

Phase 1 public API does not expose tool registration.

Current integration point is internal wiring in `agent.go` where `tools.NewRetrievalTool(...)` is passed to `graph.NewReactRunner(...)`. To add custom tools today, extend this wiring in a fork/internal change and keep retrieval tool compatibility.

