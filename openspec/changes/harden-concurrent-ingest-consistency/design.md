## Context

`ChromemStore.Upsert` currently performs a multi-step write sequence: validate batch, add/overwrite docs, then query/delete stale IDs by parent. Even with per-parent stale cleanup, overlapping upserts on the same store can still interleave those phases unless the store itself serializes them.

## Goals / Non-Goals

**Goals:**

- Make store-level upserts deterministic under concurrency.
- Preserve the existing upsert semantics and test coverage.
- Add a concurrency regression test that proves a second upsert cannot overtake the first while the first is mid-flight.

**Non-Goals:**

- Cross-process coordination.
- More granular per-parent locking.
- Public API changes.

## Decisions

### Add a store-local upsert mutex

`ChromemStore` will use an internal mutex to serialize `Upsert` calls.

Why:
- Smallest safe change.
- Eliminates mixed-state writes inside one process.
- Keeps the storage contract unchanged for upper layers.

Alternative considered:
- Per-parent lock map. Rejected for now because it is more complex and unnecessary for current Phase 1 throughput requirements.

## Risks / Trade-offs

- [Reduced concurrent ingest throughput] → acceptable for Phase 1 because correctness is more important than parallel write throughput.
- [Test seam complexity] → reuse the existing `afterAddHook` seam to assert serialization rather than introducing more hooks.

## Migration Plan

1. Add change artifacts.
2. Serialize `Upsert` with a store-local mutex.
3. Add regression test.
4. Run normal repository validation.

## Open Questions

- None.
