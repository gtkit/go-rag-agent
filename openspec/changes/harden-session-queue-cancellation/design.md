## Context

The library already serializes same-session `Ask` and `AskStream` calls using a session-local execution mutex. That ensures state mutation does not interleave, but it also means queued work waits on a blocking `sync.Mutex` that cannot observe `ctx.Done()`. If the active request is slow, a queued request can remain blocked even after the caller has canceled it.

## Goals / Non-Goals

**Goals:**

- Preserve same-session serialization.
- Allow queued `Ask` / `AskStream` requests to fail promptly with context cancellation before they begin execution.
- Keep the implementation local to the session execution gate and tests.

**Non-Goals:**

- Changing retrieval/model behavior.
- Adding fairness guarantees beyond current FIFO-ish semaphore behavior.
- Changing root API signatures.

## Decisions

### Replace blocking session execution mutex with a context-aware execution slot

The session execution gate will use a lazily initialized channel-based semaphore instead of a plain `sync.Mutex`. A queued request will `select` between acquiring the execution token and `ctx.Done()`.

Why this approach:
- Keeps serialization simple.
- Allows cancellation before execution starts.
- Avoids broader changes to root lifecycle or state-mutation logic.

Alternative considered:
- `sync.Cond` plus explicit waiting loops. Rejected because it is more error-prone and offers no benefit over a single-token channel for this use case.

### Keep state mutation locking separate

The session state mutex remains responsible for `executing`, `pendingClear`, and `pendingClose`. The new execution slot only controls admission/serialization of session work.

Why this approach:
- Matches the already-established split between execution serialization and state mutation.
- Minimizes change scope.

## Risks / Trade-offs

- [Semaphore initialization races] → use `sync.Once` to initialize the execution channel exactly once.
- [Queued cancellation semantics regression] → add focused tests that hold one request active while another queued request is canceled.
- [Behavioral drift between sync and stream paths] → apply the same execution-slot acquisition in both `Ask` and `AskStream`.

## Migration Plan

1. Add the change artifacts.
2. Replace the blocking execution mutex with a context-aware execution slot.
3. Add sync/stream cancellation regression tests.
4. Run normal repository validation.

No deployment or data migration is required.

## Open Questions

- None.
