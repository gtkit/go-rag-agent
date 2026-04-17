## Why

The current library only ingests local `.txt` and `.md` files, which is too restrictive for real personal knowledge bases where PDF is a common format. We need a local PDF ingestion path so users can index ordinary text PDFs without pre-converting them by hand.

## What Changes

- Extend local knowledge source resolution to accept `.pdf` files.
- Extend file loading to extract text from ordinary local PDFs and feed that text into the existing chunk/embed/upsert pipeline.
- Add regression tests for PDF source resolution and PDF text extraction.
- Update documentation to state the new `.pdf` support and its scope.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `embedded-rag-library`: local knowledge ingestion now supports `.pdf` files in addition to `.txt` and `.md`.

## Impact

- Affects root source resolution, rag file loading, tests, and module dependencies.
- Adds a PDF reader dependency and a test-only PDF generator dependency.
- Does not change the public API shape.
