## Why

The runtime extension work is locally verified, but production readiness still needs explicit validation entrypoints and deployment materials that operators can run against real providers without committing secrets.

## What Changes

- Add opt-in external integration validation contracts for live OpenAI-compatible chat/embedding and pgvector deployments.
- Add production deployment assets for environment configuration, Docker Compose based dependency validation, and operator-facing runbooks.
- Update README with a validation command matrix and explicit skip semantics for missing external credentials.
- No default Shell, database execution, cron, or multi-agent orchestration tools are added.

## Capabilities

### New Capabilities
- `production-validation-assets`: Covers production validation entrypoints, environment templates, deployment sample files, and operator documentation.

### Modified Capabilities
- `example-tests`: Production validation assets extend the example/test contract with explicit externally gated integration tests and deployment examples.

## Impact

- Affected areas: `examples/`, `docs/`, optional integration tests, README, and deployment template files.
- Runtime API compatibility: no breaking changes.
- External systems: OpenAI-compatible providers and PostgreSQL/pgvector are only contacted when explicit environment variables are present.
