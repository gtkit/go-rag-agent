## Context

The ingestion pipeline is already built around local source resolution followed by file loading, chunking, embedding, and storage. PDF support fits naturally at the file-loading layer: source resolution needs to admit `.pdf`, and `LoadFile` needs a text extraction branch for PDF documents. The goal is ordinary text-based PDFs only; scanned-image PDFs and OCR are explicitly out of scope.

## Goals / Non-Goals

**Goals:**

- Support local regular `.pdf` files in `FileSource` and `DirSource`.
- Extract textual page content from ordinary PDFs and reuse the existing chunk/embed/upsert pipeline unchanged.
- Keep the implementation local and deterministic.

**Non-Goals:**

- OCR or scanned-image recognition.
- Remote PDF ingestion.
- Password-protected or highly malformed PDF recovery beyond normal parser errors.

## Decisions

### Use `rsc.io/pdf` for extraction

Use `rsc.io/pdf v0.1.1` as the runtime reader because it is tagged and sufficient for extracting page text content.

Why:
- Stable tagged module.
- No need for a large document-processing stack.
- Works directly with local files.

Alternative considered:
- `github.com/ledongthuc/pdf`: rejected because it has no tagged stable version in module discovery.

### Generate test fixtures with `github.com/jung-kurt/gofpdf`

Use `gofpdf` only in tests to create small, deterministic text PDFs.

Why:
- Avoids committing binary fixture files.
- Keeps tests local and reproducible.

## Risks / Trade-offs

- [PDF extraction order differs from author intent] → collect page text in library-provided page/text order and keep expectations small and content-focused in tests.
- [Non-text PDFs yield poor output] → document that Phase 1 supports ordinary text PDFs only.
- [Extra dependencies increase module size] → keep the runtime reader small and the generator test-only.

## Migration Plan

1. Add the PDF dependencies.
2. Extend source resolution to admit `.pdf`.
3. Extend `LoadFile` to extract text from PDF files.
4. Add PDF resolution/extraction tests.
5. Update README.

## Open Questions

- None.
