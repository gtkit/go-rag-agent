## Context

The current machine does not have a `youdaonote` CLI installed, and the old Youdao cloud OpenAPI path is not a practical default because new access is restricted. The best fit is a local bridge source that calls a user-installed CLI/Skills command to export notes into a temporary directory and then hands that directory to the existing local ingestion pipeline.

## Goals / Non-Goals

**Goals:**

- Add a Youdao bridge source to the root package without changing the core ingestion pipeline.
- Require only a local CLI dependency, not direct cloud API credentials.
- Fail clearly when the local CLI is unavailable or returns an error.

**Non-Goals:**

- Direct integration with deprecated/restricted Youdao OpenAPI.
- Reverse-engineering private protocols.
- Managing Youdao authentication from inside the library.

## Decisions

### Use local CLI bridge execution

The library will define a Youdao source that runs a local `youdaonote` command, exports notes into a temporary directory, and then resolves those files through the existing local source path.

Why:
- Reuses the existing local `.md/.txt/.pdf` ingestion path.
- Keeps credentials and note-app auth outside the library.
- Aligns with the practical recommendation to use local Skills/CLI tooling instead of new OpenAPI onboarding.

### Keep bridge execution configurable but minimal

The first version will support a command name, argument template, and an export target notion that writes into a temp directory. It will not attempt to support every possible CLI shape.

Why:
- Smallest practical feature that still works.
- Easier to test with a fake local command.

## Risks / Trade-offs

- [CLI shape varies from machine to machine] → keep the source config explicit and document the expected command contract.
- [CLI missing on host] → return a clear error that names the missing executable.
- [Bridge export creates unsupported file types] → reuse current local source filtering so unsupported files are ignored or rejected through existing logic.

## Migration Plan

1. Add change artifacts.
2. Add a root `YoudaoNoteSource` bridge source and tests.
3. Wire `AddKnowledge` to transparently support it through the existing source resolution flow.
4. Update README.

## Open Questions

- None.
