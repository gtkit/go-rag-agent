# go-rag-agent

`go-rag-agent` is a Go 1.26.2 RAG runtime with local ingestion, chromem-backed vector retrieval, session memory, and OpenAI-compatible chat/embedding adapters.

## Installation

Current module path is `my-gtkit-package/go-rag-agent`, which is used as a local/private import path in this repository.

Use it from a checked-out workspace or an internal VCS path that provides the same module path.

Example (consumer module using local checkout):

```go
require my-gtkit-package/go-rag-agent v0.0.0

replace my-gtkit-package/go-rag-agent => ../go-rag-agent
```

## Quick Start

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

## Config Reference

Required:
- `ChatModel`
- `ChatBaseURL`
- `ChatAPIKey`
- `EmbeddingModel`

Optional fields (defaults applied by `New` / `Validate`):
- `TopK` (`5`)
- `ChunkSize` (`1000`)
- `MaxHistoryRounds` (`8`)
- `MaxIterations` (`3`)
- `RequestTimeout` (`30s`)

Validation notes:
- Empty `DataDir` keeps in-memory vector storage (no forced `"."` persistence path).
- `SimilarityThreshold: 0` is preserved as-is and keeps non-negative-similarity hits; use a negative value if you want to include negative-similarity matches too.
- `ChunkSize` must fit within the current assembled evidence budget (`<= 4000` runes).
- `ChunkOverlap` must be `>=0` and `< ChunkSize`.
- `EnableHybridSearch` and `EnableRerank` are rejected in phase 1.
- `MaxToolCalls` exists in `Config` but is reserved in phase 1 runtime wiring.

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
