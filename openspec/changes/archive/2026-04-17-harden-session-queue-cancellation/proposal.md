## Why

The current session execution gate serializes same-session work correctly, but a queued request can still block indefinitely on the execution lock even after its context is canceled. That makes cancellation semantics weaker than the Phase 1 execution contract and can waste time and resources under load.

## What Changes

- Make same-session execution admission context-aware so queued `Ask` and `AskStream` calls return promptly when their context is canceled before they start running.
- Add regression tests covering queued-request cancellation while another session operation is in flight.
- Keep the existing same-session serialization and lifecycle semantics intact.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `session-memory-execution`: strengthen same-session execution semantics so queued work respects context cancellation before execution starts.

## Impact

- Affects session execution coordination in the root runtime (`session.go`) and related streaming/sync tests.
- Does not change public API surface.
- Does not introduce new dependencies.
