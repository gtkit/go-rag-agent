## Why

The current storage adapter can interleave concurrent `Upsert` calls against the same `ChromemStore`, which creates mixed-state risks during parent-level replacement and stale-chunk cleanup. We need deterministic ingest semantics so concurrent knowledge updates cannot partially delete each other’s chunks.

## What Changes

- Serialize `ChromemStore.Upsert` executions within a store instance.
- Add regression coverage for overlapping upserts against the same store.
- Clarify the actual `SimilarityThreshold` behavior in docs.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `embedded-rag-library`: embedded storage ingestion now guarantees deterministic per-store upsert sequencing under concurrent writes.

## Impact

- Affects `internal/storage/chromem_store.go` and its tests.
- Updates README wording for threshold semantics.
- Does not change the public API shape.
