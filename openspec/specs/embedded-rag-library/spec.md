## Purpose

Define the library-first RAG runtime API for embedded use in Go applications.

## Requirements

### Requirement: Library-first agent construction
The system SHALL provide a small root-package API that allows Go applications to construct an agent, obtain sessions, ingest knowledge sources, ask synchronous questions, stream answers, and release resources without running a standalone HTTP service.

#### Scenario: Create agent with valid Phase 1 config
- **WHEN** a caller constructs an agent with valid chat model, embedding model, and timeout configuration
- **THEN** the library creates an `Agent` instance ready to ingest knowledge and serve sessions

#### Scenario: Reject unsupported Phase 2 flags during Phase 1
- **WHEN** a caller enables Phase 2-only behavior such as hybrid retrieval or reranking in Phase 1 configuration
- **THEN** the library MUST reject construction with an invalid configuration error

### Requirement: Local knowledge ingestion
The system SHALL ingest `.txt` and `.md` knowledge sources from local files and directories, split them into deterministic chunks, generate embeddings, and persist chunk content plus citation metadata into embedded storage.

#### Scenario: Ingest supported file and directory sources
- **WHEN** a caller passes a supported local file or directory source to `AddKnowledge`
- **THEN** the library loads each `.txt` and `.md` file, chunks its content, embeds the chunks, and stores them with source path, title, chunk ID, and offset metadata

#### Scenario: Reject unsupported knowledge source types
- **WHEN** a caller passes a non-`.txt` or non-`.md` source
- **THEN** the library MUST return an unsupported-source error

### Requirement: Retrieval-backed synchronous answers
The system SHALL answer synchronous queries using retrieved evidence from embedded storage and SHALL return citations associated with the selected evidence.

#### Scenario: Return answer with citations
- **WHEN** a session asks a question after relevant knowledge has been ingested
- **THEN** the library retrieves bounded evidence, generates an answer, and returns both answer text and citations for the supporting chunks

#### Scenario: Report insufficient evidence conservatively
- **WHEN** the retrieved evidence is empty or too weak to assemble usable context
- **THEN** the library MUST return an insufficient-evidence or context-assembly error rather than silently fabricating support

### Requirement: Callback-based streaming answers
The system SHALL provide a callback-based streaming API that emits retrieval and tool lifecycle events, answer chunks, citations, error events, and a terminal done event.

#### Scenario: Stream answer chunks and completion
- **WHEN** a session calls `AskStream` for a query that completes successfully
- **THEN** the library emits zero or more answer chunk events followed by a done event

#### Scenario: Surface citations during streaming
- **WHEN** relevant evidence is selected for a streaming answer
- **THEN** the library emits citation events associated with the supporting chunks before or during answer streaming

#### Scenario: Emitter panic is converted to error
- **WHEN** the caller’s streaming emitter panics while the library is emitting stream events
- **THEN** the library MUST recover the panic, return an ordinary error to the caller, and still finalize session execution state
