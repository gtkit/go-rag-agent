## MODIFIED Requirements

### Requirement: Local knowledge ingestion
The system SHALL ingest `.txt` and `.md` knowledge sources from local files and directories, split them into deterministic chunks, generate embeddings, and persist chunk content plus citation metadata into embedded storage.

#### Scenario: Ingest supported file and directory sources
- **WHEN** a caller passes a supported local file or directory source to `AddKnowledge`
- **THEN** the library loads each `.txt` and `.md` file, chunks its content, embeds the chunks, and stores them with source path, title, chunk ID, and offset metadata

#### Scenario: Reject unsupported knowledge source types
- **WHEN** a caller passes a non-`.txt` or non-`.md` source
- **THEN** the library MUST return an unsupported-source error

#### Scenario: Concurrent upserts on one store are serialized
- **WHEN** two knowledge ingestion operations target the same store concurrently
- **THEN** the store MUST execute their upsert phases one at a time so chunk replacement and stale cleanup cannot interleave into a mixed state
