## Overview

This change adds production validation and deployment assets without turning the SDK into a managed platform. The validation entrypoints must be safe by default: local CI runs remain deterministic, while operators can opt in to real provider/database checks by setting environment variables.

## Decisions

### Opt-in integration tests

Use Go integration tests that skip when required environment variables are missing. This preserves normal `go test ./...` behavior and makes real external checks auditable:

- `RAGAGENT_INTEGRATION_LIVE=1` enables live provider tests.
- `RAGAGENT_CHAT_MODEL`, `RAGAGENT_CHAT_BASE_URL`, `RAGAGENT_CHAT_API_KEY` configure live chat.
- `RAGAGENT_EMBEDDING_MODEL`, optional `RAGAGENT_EMBEDDING_BASE_URL`, and optional `RAGAGENT_EMBEDDING_API_KEY` configure embeddings.
- Existing `RAGAGENT_PGVECTOR_TEST_DSN` continues to gate pgvector tests.

### Deployment assets are templates

Provide checked-in templates only:

- `.env.production.example` with variable names and safe placeholder descriptions.
- `deploy/compose/pgvector.compose.yml` for local pgvector dependency validation.
- `docs/production.md` for command matrix, rollout notes, and operational checks.

No real credentials, model account IDs, tenant names, or production DSNs are stored.

### No high-risk built-in tools

This change does not add default Shell, SQL execution, cron, or broad orchestration tools. Those remain caller-supplied integrations requiring separate authorization, audit, timeout, and sandbox design.

## Risks

- Live integration tests may be flaky due to network/provider limits. They are opt-in and documented as external validation, not mandatory local CI.
- Docker Compose availability varies by environment. Compose assets are examples; unit tests validate their presence and required service configuration without requiring Docker.

## Validation

- Unit tests verify env parsing, skip behavior, and deployment asset contents.
- Existing full validation remains required: lint, vet, coverage, race, OpenSpec strict validation.
