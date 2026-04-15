# Go RAG Agent Phase 1 Design

## Status

- Date: 2026-04-15
- Scope: Phase 1 core library only
- Approval: interactive design approved in chat before writing this spec

## Goal

Implement `go-rag-agent` as a production-grade Go library for embedding RAG agents into Go applications without requiring a standalone HTTP service or external stateful infrastructure by default.

Phase 1 must deliver a small, stable public API, local knowledge ingestion for `.txt` and `.md`, bounded session memory, evidence-first retrieval, synchronous and streaming answers, and concurrency-safe multi-session execution.

## Non-Goals

Phase 1 does not implement:

- standalone web services or admin UI
- distributed actors, remote messaging, or workflow orchestration
- Redis, PostgreSQL, Milvus, Kafka, or any required external infrastructure
- multimodal support
- hybrid retrieval
- reranking
- rolling history summarization
- long-term memory persistence
- tracing backends, circuit breakers, or rate limiting as mandatory runtime components

## Design Decisions

### Recommended Approach

Use a library-first kernel with a narrow root-package API and internal layered subsystems for orchestration, retrieval, memory, tools, LLM access, storage, and telemetry hooks.

This is preferred over exposing low-level Eino graph construction directly because:

- it matches the prompt requirement for a small and stable public API
- it keeps Eino-specific orchestration details internal
- it makes testing easier with fakes for retrieval, storage, and model clients
- it avoids overcommitting to advanced persistence and retrieval features in the first release

### Core Design Constraints

- keep the deliverable as a Go library
- keep all session state explicit rather than hiding it in `context.Context`
- serialize state mutation within the same session
- allow concurrent execution across different sessions
- use callback-style streaming rather than exposing raw channels
- use `github.com/gtkit/json` for project-owned JSON handling
- inject logger and callbacks instead of hardcoding logging or telemetry stacks
- use current stable Eino APIs during implementation, without fabricating API names in advance

## Public API

Phase 1 keeps the root package small and stable.

```go
type Agent struct{}

type Config struct {
    ChatModel           string
    ChatBaseURL         string
    ChatAPIKey          string
    EmbeddingModel      string
    EmbeddingBaseURL    string
    EmbeddingAPIKey     string

    DataDir             string
    TopK                int
    SimilarityThreshold float32
    ChunkSize           int
    ChunkOverlap        int
    MaxHistoryRounds    int
    MaxToolCalls        int
    MaxIterations       int
    RequestTimeout      time.Duration
    EnableHybridSearch  bool
    EnableRerank        bool
    Logger              Logger
    Callbacks           []Callback
}

type Session struct{}

func New(cfg Config) (*Agent, error)
func (a *Agent) GetSession(id string) *Session
func (a *Agent) AddKnowledge(ctx context.Context, src KnowledgeSource) error
func (a *Agent) Close() error

func (s *Session) Ask(ctx context.Context, query string) (Answer, error)
func (s *Session) AskStream(ctx context.Context, query string, emit func(StreamEvent) error) error
func (s *Session) ClearHistory(ctx context.Context) error
func (s *Session) Close() error
```

### Supporting Public Types

Phase 1 will expose small supporting types:

- `Answer` with `Text` and `Citations`
- `Citation` with enough source metadata for answer attribution
- `StreamEvent` with event type, content, tool name, step, error, and timestamp
- `KnowledgeSource` abstraction with local-only helpers such as `FileSource` and `DirSource`
- `Logger` as a narrow injected interface compatible with `github.com/gtkit/logger`
- `Callback` hook interface for retrieval, tool, and model lifecycle events

`EnableHybridSearch` and `EnableRerank` remain in `Config` for forward compatibility, but Phase 1 treats them as disabled features and validates or ignores them deterministically rather than partially implementing them.

## Package Layout

```text
go-rag-agent/
├── agent.go
├── config.go
├── session.go
├── errors.go
├── types.go
├── doc.go
├── README.md
├── examples/
└── internal/
    ├── graph/
    ├── rag/
    ├── memory/
    ├── tools/
    ├── llm/
    ├── storage/
    └── telemetry/
```

### Root Package Responsibilities

- validate config and construct dependencies
- expose stable user-facing types and operations
- own lifecycle methods such as `Close`

### Internal Package Responsibilities

- `internal/graph`: builds the Eino execution graph for retrieval-augmented answering
- `internal/rag`: loads documents, normalizes text, chunks content, assembles prompt context
- `internal/memory`: maintains bounded per-session short-term memory
- `internal/tools`: registers and executes the builtin retrieval tool
- `internal/llm`: wraps OpenAI-compatible chat and embedding access with timeout-aware clients
- `internal/storage`: owns vector store, document store, and session store abstractions and default embedded implementations
- `internal/telemetry`: fans out callback notifications and optional lightweight metrics hooks

This layout keeps public API stable while allowing later internal extension for Phase 2 and Phase 3.

## Concurrency And Session Model

### Agent-Level State

`Agent` owns shared immutable dependencies plus a session registry. The registry maps session ID to session state and is protected for concurrent access.

### Session-Level State

Each `Session` owns:

- bounded recent conversation turns
- session metadata needed for follow-up resolution
- transient tool outputs useful for the next turn
- a serialization primitive that ensures state mutation for that session is executed one request at a time

Same-session behavior:

- `Ask`, `AskStream`, `ClearHistory`, and `Close` serialize state mutation
- follow-up turns observe the latest committed session state
- cancellation of one in-flight streaming request must release session execution promptly

Cross-session behavior:

- different sessions may ask concurrently
- shared read-only dependencies may be reused concurrently

The implementation should prefer standard-library synchronization over a third-party actor library.

## Knowledge Ingestion

### Supported Sources

Phase 1 supports local `.txt` and `.md` sources only:

- `FileSource(path string)`
- `DirSource(path string)`

`DirSource` loads eligible files from a local directory tree. Remote sources, HTTP fetch, object storage, and database ingestion are out of scope.

### Ingestion Pipeline

`AddKnowledge` executes this pipeline:

1. validate source and file type
2. read file contents with `context.Context` support where applicable
3. normalize whitespace, punctuation, and metadata needed for retrieval
4. split documents into chunks using deterministic chunk size and overlap rules
5. generate embeddings for each chunk
6. persist chunks, embeddings, and citation metadata into the embedded store

### Metadata Requirements

Each chunk record should preserve enough metadata for citations and later filtering:

- source path
- document title or derived file name
- chunk ID
- optional parent document ID
- byte or line offset where practical

Phase 1 preprocesses documents once at ingestion time and never rechunks on every query.

## Retrieval Pipeline

### Retrieval Behavior

Phase 1 retrieval is vector-first and evidence-oriented:

1. normalize the current query
2. inspect recent session turns to determine whether follow-up resolution is needed
3. rewrite underspecified follow-up queries into standalone retrieval queries when confidence is sufficient
4. run bounded vector search against embedded chunk storage
5. deduplicate near-identical or overlapping chunks
6. assemble a bounded evidence context with citations

### Retrieval Quality Rules

- prefer fewer high-signal chunks over many weak chunks
- cap retrieved context by chunk count and prompt budget
- preserve source metadata for answer citations
- prefer retrieved evidence over model prior knowledge
- explicitly allow abstention when evidence is insufficient

Hybrid retrieval, reranking, dynamic retrieval budgets, and caching remain Phase 2 work.

## Memory Design

### Phase 1 Short-Term Memory

Phase 1 implements bounded short-term memory only.

Stored session memory includes:

- recent conversation turns
- current user goal when detectable from the recent conversation
- unresolved conversational references needed for follow-up turns
- recent tool outputs that are likely to matter for the next turn

Memory rules:

- do not append the entire conversation history to every prompt
- keep a bounded recent-turn window controlled by `MaxHistoryRounds`
- trim oldest turns deterministically when the budget is exceeded
- do not implement rolling summary in Phase 1

### Long-Term Memory

Long-term memory is explicitly deferred. Internal interfaces may be shaped to support it later, but there is no Phase 1 persistence, scoring, or retrieval of long-term memory.

## Generation Pipeline

### Ask

`Session.Ask` runs a controlled request pipeline:

1. validate session state and request inputs
2. resolve follow-up context from short-term memory if needed
3. retrieve bounded evidence through the builtin retrieval tool
4. build a prompt from recent turns, evidence context, and current query
5. execute the internal Eino graph with bounded iteration and tool-call limits
6. return `Answer` with final text and citations
7. update session memory after successful completion

### AskStream

`Session.AskStream` uses the same retrieval and control flow but streams answer generation through a callback:

- emit operational events such as retrieval start, retrieval end, tool start, tool end, answer chunk, error, and done
- stop immediately if the callback returns an error
- stop promptly when `ctx.Done()` fires
- close all downstream resources and goroutines before returning

Callback-style streaming is chosen to keep backpressure explicit and avoid channel lifecycle ambiguity in public API.

### Reasoning And Tool Budgets

Phase 1 enforces:

- maximum total iterations per request from `MaxIterations`
- maximum retrieval tool calls per request from `MaxToolCalls`
- per-call timeouts for model and tool work derived from `RequestTimeout`

The library does not expose chain-of-thought text.

## LLM And Embedding Access

### Provider Scope

Phase 1 supports OpenAI-compatible chat and embedding endpoints only. Separate chat and embedding models, base URLs, and API keys are configured in `Config`.

### Client Rules

- all outbound requests must respect `context.Context`
- model and embedding calls must use explicit timeout handling
- provider clients should be reused safely
- request and response handling in project-owned code must use `github.com/gtkit/json`
- normal runtime failures return typed or wrapped errors, never panics

Implementation should verify the current stable Eino and Eino OpenAI extension APIs at coding time instead of relying on guessed signatures.

## Storage Design

### Embedded Default

Phase 1 uses embedded storage with no required external service:

- `chromem-go` for vector similarity storage
- lightweight in-process document metadata storage behind narrow interfaces
- in-memory session state storage owned by `Agent`

### Internal Interfaces

Storage remains abstracted behind internal interfaces:

- vector store
- document store
- session state store

This keeps Phase 1 small while leaving room for optional persistence in later phases.

## Errors And Reliability

### Sentinel Errors

Phase 1 should define typed sentinel errors for major failure classes, including:

- invalid config
- unsupported knowledge source
- session closed
- agent closed
- context assembly failure
- insufficient evidence

### Reliability Rules

- use `fmt.Errorf("...: %w", err)` for wrapping
- use `errors.Is` and `errors.As` for classification
- expose explicit `Close` semantics for agent and session resources
- do not panic for normal runtime failures

Retry, circuit breaker, and rate limiting controls are optional future work and must not complicate Phase 1.

## Observability

Phase 1 exposes lightweight hooks rather than a mandatory telemetry stack.

```go
type Callback interface {
    OnRetrieveStart(ctx context.Context, query string)
    OnRetrieveEnd(ctx context.Context, resultCount int, err error)
    OnToolStart(ctx context.Context, tool string)
    OnToolEnd(ctx context.Context, tool string, err error)
    OnModelStart(ctx context.Context, model string)
    OnModelEnd(ctx context.Context, model string, err error)
}
```

Logging rules:

- accept an injected logger through `Config`
- default to a no-op logger when none is provided
- avoid hardcoded global logging
- avoid logging full prompts or secrets

## Testing Strategy

Phase 1 requires automated tests before or alongside implementation for these areas:

- config validation
- chunking behavior
- retrieval ranking and bounded result assembly
- session isolation across different session IDs
- same-session serialized state mutation
- streaming cancellation behavior
- no goroutine leaks on streaming paths
- race detection with `go test -race`

### Test Style

- table-driven tests for unit coverage
- fakes or mocks for LLM, embeddings, vector store, retriever, and tool execution
- deterministic assertions for cancellation, citation assembly, and memory trimming

### Benchmarks

Add benchmarks for:

- ingestion throughput
- retrieval latency
- prompt assembly cost
- streaming overhead

## Deliverables

Phase 1 deliverables:

- complete Go module source for the library
- root package API and internal implementation
- `README.md`
- `examples/`
- unit tests
- benchmarks for key paths

### README Requirements

`README.md` must include:

- installation
- quick start
- config reference
- ingestion workflow
- session isolation model
- streaming behavior
- retrieval latency design
- generation latency design
- custom tool integration approach

## Implementation Notes

- examples in this repository should use the current module path `my-gtkit-package/go-rag-agent`
- exported API comments and `example_test.go` are required for GoDoc quality
- implementation should build the smallest correct Phase 1 library before any Phase 2 optimizations

## Accepted Scope Summary

This design is explicitly approved for:

- library-first root API
- internal layered architecture
- local `.txt` and `.md` ingestion through `FileSource` and `DirSource`
- vector-first retrieval with evidence-first answering
- bounded short-term session memory without rolling summary
- callback-based streaming with cancellation-safe execution
- concurrency-safe session isolation
- tests and benchmarks required by the prompt

This design explicitly defers:

- hybrid retrieval
- reranking
- history summarization
- long-term memory persistence
- external infrastructure dependencies
- production extras beyond lightweight hooks and explicit error handling
