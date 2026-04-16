## Context

This repository is starting from prompt documents and workflow scaffolding rather than an existing library implementation. The required deliverable is an embeddable Go library, not an application template, and it must use Eino as the orchestration layer while defaulting to zero external stateful infrastructure. The current Phase 1 scope is intentionally narrower than the full prompt: no hybrid retrieval, reranking, rolling summaries, long-term memory persistence, or standalone HTTP runtime.

## Goals / Non-Goals

**Goals:**

- Provide a small stable root-package API for agent construction, session access, ingestion, ask, streaming, and cleanup.
- Implement local `.txt` and `.md` ingestion, chunking, embeddings, embedded vector search, citation-aware retrieval, and evidence-first answering.
- Implement bounded per-session short-term memory, follow-up query rewriting, same-session serialized mutation, and prompt cancellation-safe streaming.
- Keep implementation testable with narrow internal interfaces and fake-friendly boundaries.

**Non-Goals:**

- Hybrid retrieval, reranking, rolling history summarization, or long-term memory persistence.
- External databases, queues, cluster actors, or distributed orchestration.
- HTTP handlers, admin UI, or app scaffolding.
- Broad public extension APIs beyond the Phase 1 root API.

## Decisions

### 1. Root-package API with internal layered implementation

The root package will expose `Config`, `Agent`, `Session`, `Answer`, `Citation`, `StreamEvent`, and source helpers. Implementation details stay under `internal/graph`, `internal/llm`, `internal/memory`, `internal/rag`, `internal/storage`, `internal/tools`, and `internal/telemetry`.

Rationale:
- Keeps the public API small and stable.
- Preserves the ability to change orchestration and storage internals later without breaking callers.
- Matches the approved Phase 1 design and library-first requirement.

Alternatives considered:
- Exposing Eino graph construction directly: rejected because it would leak orchestration details into the public API and make stability harder.
- Building an app-style layered service/handler/repository runtime: rejected because the deliverable must remain a library.

### 2. OpenAI-compatible integration through Eino-ext packages resolved at implementation time

The implementation will use `github.com/cloudwego/eino-ext/components/model/openai@latest` and `github.com/cloudwego/eino-ext/components/embedding/openai@latest`, letting Go resolve a compatible `github.com/cloudwego/eino` version instead of hardcoding a stale guess.

Rationale:
- The prompt requires current stable Eino APIs and explicitly forbids stale or fabricated signatures.
- Eino-ext package versions evolve independently; solver-chosen compatible versions are safer than freezing an assumed pair.

Alternatives considered:
- Direct raw HTTP client implementation against OpenAI-compatible endpoints: rejected because Eino must be the orchestration layer and this would duplicate model integration work.

### 3. Embedded vector storage via `chromem-go`

Phase 1 uses `chromem-go` as the default vector store with optional persistent directory backing under the configured data directory.

Rationale:
- Satisfies the zero-external-infrastructure requirement.
- Supports query-by-embedding, local persistence, and simple metadata storage suitable for citations.
- Keeps retrieval fast and embeddable.

Alternatives considered:
- External vector databases: rejected because they violate the default runtime constraint.
- In-memory-only custom vector store: rejected because `chromem-go` already solves the needed Phase 1 storage problem.

### 4. Short-term memory only, serialized by per-session locking

Each session will maintain bounded recent turns and serialize mutation with a per-session mutex. Follow-up rewriting will use recent user turns from this short-term history.

Rationale:
- Meets the concurrency requirement that the same session is serialized while different sessions may run concurrently.
- Keeps session state explicit and local rather than hidden in `context.Context`.
- Avoids premature complexity from actor frameworks or long-term memory persistence.

Alternatives considered:
- Mailbox goroutine per session: acceptable in theory, but rejected for Phase 1 because mutex-based serialization is simpler and easier to test.

### 5. Callback-based streaming with event translation at the root package boundary

Internal graph streaming will emit minimal graph events, while the root package converts them into `StreamEvent` values and emits citations, tool events, answer chunks, error events, and done events through a callback.

Rationale:
- Keeps backpressure explicit and avoids public channel lifecycle ambiguity.
- Allows the root package to preserve the approved event model while keeping internal graph interfaces narrow.
- Makes cancellation handling and goroutine cleanup easier to reason about.

Alternatives considered:
- Public streaming channels: rejected because cancellation and close semantics are harder to document and test correctly.

### 6. Vector-first retrieval with deterministic context assembly

Phase 1 retrieval will normalize the query, rewrite referential follow-up questions when recent history allows, query the vector store with the embedded query, deduplicate repeated hits, and assemble a bounded context block plus citations.

Rationale:
- Matches the approved Phase 1 scope without partially implementing hybrid retrieval or reranking.
- Keeps prompt assembly deterministic and testable.
- Supports evidence-first answers and explicit abstention when support is weak.

Alternatives considered:
- Hybrid lexical plus vector retrieval: deferred to Phase 2.
- Unbounded context concatenation: rejected because it violates prompt budget constraints and hurts generation latency.

## Risks / Trade-offs

- [Eino-ext package drift] → Resolve compatible package versions during implementation and verify imports against downloaded module source before coding against them.
- [Circular dependency pressure between root and internal packages] → Keep graph interfaces internal and return root-level answer/citation structures from the root package, not from `internal/*`.
- [Streaming event duplication] → Emit retrieval/tool/model lifecycle events from the root package in one place and keep graph event semantics minimal.
- [Persistent and in-memory state divergence] → Restrict Phase 1 persistence to vector storage only and keep session state in memory.
- [Testing false positives for cancellation] → Include dedicated streaming cancellation and goroutine-count tests under `go test -race`.

## Migration Plan

1. Create the Phase 1 OpenSpec artifacts and tasks.
2. Add the initial dependencies and public contracts.
3. Implement internal ingestion, storage, memory, orchestration, and session flows behind the root API.
4. Add docs, examples, and tests.
5. Run lint, vet, race tests, benchmarks, and strict OpenSpec validation.

Rollback strategy:
- Revert the feature branch or the individual commits if implementation proves unstable. There is no production migration because this is the first library implementation.

## Open Questions

- None for Phase 1. The approved scope is intentionally constrained, and Phase 2 features remain explicitly deferred.
