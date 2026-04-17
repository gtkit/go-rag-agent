## MODIFIED Requirements

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
