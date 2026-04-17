## Why

The library currently invokes telemetry and stream-emitter callbacks inline. If a callback panics, request finalization may be interrupted and session state can be left in a partially updated state. We need callback panic handling that preserves runtime cleanup and returns a regular error to callers.

## What Changes

- Recover panics from user-provided streaming emitters and telemetry callbacks at the root runtime boundary.
- Convert recovered panics into ordinary errors so sync and streaming operations can finish cleanup deterministically.
- Add regression tests for callback panic recovery in both telemetry and stream-emitter paths.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `embedded-rag-library`: callback-based streaming and runtime callback hooks now recover panic boundaries instead of leaking panics through the library surface.
- `session-memory-execution`: in-flight session execution now finalizes cleanly even when callbacks panic.

## Impact

- Affects root runtime callback execution in `agent.go`.
- Affects streaming tests and agent-level lifecycle tests.
- No new dependencies.
