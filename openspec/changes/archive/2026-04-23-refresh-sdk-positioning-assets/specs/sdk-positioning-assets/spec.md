## ADDED Requirements

### Requirement: README SHALL state the SDK positioning clearly
The repository SHALL present `go-rag-agent` as a Go SDK / embeddable runtime on the README landing section, and SHALL distinguish that positioning from control-plane or platform products.

#### Scenario: README landing section explains scope
- **WHEN** a reader opens the repository README from the top
- **THEN** the README MUST describe what the project is, what it is not, and which integration scenarios it targets

### Requirement: Repository SHALL provide scenario-oriented integration examples
The repository SHALL provide runnable example entrypoints that cover the minimum embedded setup, existing-service embedding, and pgvector-oriented deployment wiring.

#### Scenario: Examples directory covers three primary scenarios
- **WHEN** a reader opens the repository examples
- **THEN** the repository MUST provide example programs for basic embedded usage, service integration, and pgvector-backed deployment

### Requirement: Repository SHALL provide reproducible baseline artifacts
The repository SHALL provide a committed benchmark / eval baseline document, sample output files, and a command entrypoint that regenerates those artifacts from repository fixtures.

#### Scenario: Baseline assets can be regenerated from the repository
- **WHEN** a maintainer runs the documented baseline generation command from the repository root
- **THEN** the command MUST produce the documented eval report and trace summary files using stable repository fixtures

### Requirement: Repository SHALL provide a release gate
The repository SHALL provide a single local verification script and CI workflow that run OpenSpec validation, Go vet, golangci-lint, race tests, and SDK positioning baseline regeneration.

#### Scenario: Release gate verifies repository quality checks
- **WHEN** a maintainer runs the repository verification script
- **THEN** the script MUST run OpenSpec validation, Go vet, golangci-lint, race tests, and baseline regeneration before release
*** Add File: openspec/specs/sdk-positioning-assets/spec.md
## 目的

定义仓库级定位文档、场景化示例、可复现 baseline 资产和 release gate。

## 要求

### Requirement: README SHALL state the SDK positioning clearly
The repository SHALL present `go-rag-agent` as a Go SDK / embeddable runtime on the README landing section, and SHALL distinguish that positioning from control-plane or platform products.

#### Scenario: README landing section explains scope
- **WHEN** a reader opens the repository README from the top
- **THEN** the README MUST describe what the project is, what it is not, and which integration scenarios it targets

### Requirement: Repository SHALL provide scenario-oriented integration examples
The repository SHALL provide runnable example entrypoints that cover the minimum embedded setup, existing-service embedding, and pgvector-oriented deployment wiring.

#### Scenario: Examples directory covers three primary scenarios
- **WHEN** a reader opens the repository examples
- **THEN** the repository MUST provide example programs for basic embedded usage, service integration, and pgvector-backed deployment

### Requirement: Repository SHALL provide reproducible baseline artifacts
The repository SHALL provide a committed benchmark / eval baseline document, sample output files, and a command entrypoint that regenerates those artifacts from repository fixtures.

#### Scenario: Baseline assets can be regenerated from the repository
- **WHEN** a maintainer runs the documented baseline generation command from the repository root
- **THEN** the command MUST produce the documented eval report and trace summary files using stable repository fixtures

### Requirement: Repository SHALL provide a release gate
The repository SHALL provide a single local verification script and CI workflow that run OpenSpec validation, Go vet, golangci-lint, race tests, and SDK positioning baseline regeneration.

#### Scenario: Release gate verifies repository quality checks
- **WHEN** a maintainer runs the repository verification script
- **THEN** the script MUST run OpenSpec validation, Go vet, golangci-lint, race tests, and baseline regeneration before release
