## MODIFIED Requirements

### Requirement: Cancellation-safe streaming execution
The system SHALL stop downstream work promptly when a streaming request is canceled or when the callback returns an error, and SHALL release internal resources before returning.

#### Scenario: Context cancellation during streaming
- **WHEN** the caller cancels the context for an in-flight `AskStream` request
- **THEN** the library stops retrieval/model streaming promptly and returns a cancellation-related error

#### Scenario: Callback stops the stream
- **WHEN** the caller’s streaming callback returns an error
- **THEN** the library stops further emission and returns that error to the caller

#### Scenario: Callback panic does not break session finalization
- **WHEN** a telemetry callback or stream-emitter callback panics during an in-flight request
- **THEN** the library MUST recover the panic, convert it into an ordinary error, and still complete the request’s session finalization path without leaving execution state stuck
