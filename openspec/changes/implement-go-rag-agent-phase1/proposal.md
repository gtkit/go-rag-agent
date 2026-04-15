## Why

The repository currently contains design prompts and workflow scaffolding but does not yet provide the promised `go-rag-agent` library. We need a production-oriented Phase 1 library now so other Go projects can import it directly and run embedded RAG flows without deploying a standalone service or external stateful infrastructure.

## What Changes

- Add a small public `ragagent` API for agent construction, session access, knowledge ingestion, synchronous answers, streaming answers, and cleanup.
- Add local `.txt` and `.md` knowledge ingestion with deterministic chunking, embeddings, embedded vector storage, and citation-preserving retrieval.
- Add OpenAI-compatible chat and embedding adapters using Eino/Eino-ext as the orchestration layer and `chromem-go` as the default embedded vector store.
- Add bounded per-session short-term memory, follow-up query resolution, same-session serialized mutation, and cancellation-safe streaming.
- Add tests, benchmarks, examples, and README documentation for the Phase 1 library.

## Capabilities

### New Capabilities

- `embedded-rag-library`: Library-first API for configuration, agent construction, local knowledge ingestion, retrieval-backed question answering, and streaming responses.
- `session-memory-execution`: Session-scoped bounded memory, follow-up resolution, same-session serialized execution, and cancellation-safe streaming behavior.

### Modified Capabilities

- None.

## Impact

- Adds the initial Go module source tree for the library root package and internal implementation packages.
- Introduces Phase 1 runtime dependencies for Eino OpenAI-compatible model and embedding components, `chromem-go`, `gtkit/json`, and `gtkit/logger`.
- Defines the initial public API surface that downstream Go applications will import.
- Adds unit tests, race-safe concurrency tests, benchmarks, examples, and OpenSpec artifacts for ongoing change tracking.
