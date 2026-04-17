## Purpose

Define session-scoped memory, follow-up resolution, serialized mutation, and cancellation-safe execution behavior.

## Requirements

### Requirement: Bounded short-term session memory
The system SHALL maintain bounded short-term memory per session and SHALL not append the entire conversation history to every prompt.

#### Scenario: Trim old turns when capacity is exceeded
- **WHEN** a session appends turns beyond the configured maximum history rounds
- **THEN** the library keeps only the most recent bounded turns in session memory

#### Scenario: Clear history explicitly
- **WHEN** a caller invokes `ClearHistory` on a live session
- **THEN** the session memory is cleared before the next query is executed

### Requirement: Follow-up query resolution
The system SHALL resolve referential follow-up queries against recent session history before retrieval whenever the recent context is sufficient to do so deterministically.

#### Scenario: Rewrite referential follow-up
- **WHEN** a user asks a referential follow-up question such as “how does it work?” after a concrete prior question
- **THEN** the library rewrites the retrieval query using recent session context before embedding and search

#### Scenario: Preserve standalone queries
- **WHEN** a user asks a standalone query without referential language
- **THEN** the library normalizes the query without adding prior session content

### Requirement: Same-session serialized mutation
The system SHALL serialize state mutation for the same session while allowing different sessions to execute concurrently.

#### Scenario: Concurrent requests for the same session
- **WHEN** two requests target the same session concurrently
- **THEN** the library executes session state mutation one at a time so that history updates observe a consistent order

#### Scenario: Concurrent requests for different sessions
- **WHEN** requests target different session IDs
- **THEN** the library MAY execute them concurrently without sharing mutable session state

#### Scenario: Queued request is canceled before execution starts
- **WHEN** one request is already executing for a session and a second request for the same session is queued behind it
- **AND** the queued request’s context is canceled before it acquires the session execution slot
- **THEN** the queued request MUST return promptly with the context cancellation error instead of waiting for the active request to finish

### Requirement: Cancellation-safe streaming execution
The system SHALL stop downstream work promptly when a streaming request is canceled or when the callback returns an error, and SHALL release internal resources before returning.

#### Scenario: Context cancellation during streaming
- **WHEN** the caller cancels the context for an in-flight `AskStream` request
- **THEN** the library stops retrieval/model streaming promptly and returns a cancellation-related error

#### Scenario: Callback stops the stream
- **WHEN** the caller’s streaming callback returns an error
- **THEN** the library stops further emission and returns that error to the caller
