## Context

The root runtime already enforces serialization and lifecycle handling around sync and streaming execution, but callback invocation is still inline and panic-unsafe. A panic from telemetry or the public streaming emitter can bypass normal return/error handling and leave cleanup paths dependent on caller-side recovery.

## Goals / Non-Goals

**Goals:**

- Recover callback panics at the root boundary.
- Convert recovered panic values into ordinary errors.
- Preserve existing event ordering and lifecycle behavior when callbacks do not panic.
- Ensure session execution finalization still runs after callback panic.

**Non-Goals:**

- Sandboxing arbitrary callback code.
- Retrying callbacks.
- Changing callback interfaces or adding recovery configuration knobs.

## Decisions

### Recover callback panics at the narrowest root boundary

Telemetry and emitter callbacks will be invoked through small helper wrappers in the root runtime that recover panics and convert them into `error` values.

Why:
- Keeps recovery localized.
- Preserves existing internal package boundaries.
- Avoids requiring callers to add their own recovery just to keep library state consistent.

### Preserve normal cleanup by returning errors instead of re-panicking

Recovered callback panics will be surfaced as ordinary errors so existing `Ask` / `AskStream` cleanup logic can complete naturally.

Why:
- Matches existing error-driven lifecycle handling.
- Prevents half-finished session state updates.

## Risks / Trade-offs

- [Recovered panic hides original stack] → include the panic value in the returned error string.
- [Behavioral change for callers expecting panic] → document the library behavior through tests and change artifacts.
- [Over-broad recovery masking internal bugs] → apply recovery only around external callback invocation sites, not around the whole request pipeline.

## Migration Plan

1. Add change artifacts.
2. Wrap telemetry and emitter callbacks with panic recovery helpers.
3. Add regression tests.
4. Run repository validation.

## Open Questions

- None.
