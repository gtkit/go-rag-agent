## ADDED Requirements

### Requirement: Youdao Note bridge import
The system SHALL provide a knowledge source that imports note content from a locally installed Youdao Note CLI bridge by exporting notes to a temporary local directory and then ingesting the exported files through the normal local pipeline.

#### Scenario: Export and ingest notes through local bridge
- **WHEN** a caller configures a Youdao Note bridge source and the local `youdaonote` command successfully exports notes into the provided temporary directory
- **THEN** the library MUST ingest the exported note files through the same local file pipeline used by `DirSource`

#### Scenario: Missing local bridge command
- **WHEN** a caller uses a Youdao Note bridge source but the configured `youdaonote` command is not installed
- **THEN** the library MUST return a clear unsupported-source style error naming the missing command

#### Scenario: Bridge export failure
- **WHEN** the local `youdaonote` export command exits with an error
- **THEN** the library MUST return that failure as an ingestion error and MUST NOT silently continue with an empty knowledge set
