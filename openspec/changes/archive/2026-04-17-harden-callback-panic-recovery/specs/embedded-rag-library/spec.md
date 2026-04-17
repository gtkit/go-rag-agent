## MODIFIED Requirements

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
