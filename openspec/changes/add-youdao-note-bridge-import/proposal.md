## Why

The library already supports local `.txt`, `.md`, and `.pdf` ingestion, but many personal knowledge bases live in note-taking tools instead of raw files. We want a practical path for importing Youdao Note content without relying on the deprecated/openly restricted cloud API route.

## What Changes

- Add a local bridge-style knowledge source for Youdao Note that shells out to a locally installed `youdaonote` CLI/Skills command.
- Import exported note content by first materializing it into a temporary local directory, then reusing the existing local ingestion pipeline.
- Return a clear unsupported/missing-prerequisite error when the required CLI is not installed or export fails.
- Document that this is a bridge to local tooling, not a direct OpenAPI integration.

## Capabilities

### New Capabilities

- `youdao-note-bridge-import`: ingest personal knowledge from a locally installed Youdao Note CLI export bridge.

### Modified Capabilities

- None.

## Impact

- Adds a new root knowledge source type and local bridge execution code.
- Adds tests around CLI detection and temp-dir bridge behavior.
- Updates README to explain Youdao bridge prerequisites and usage.
