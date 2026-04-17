## MODIFIED Requirements

### Requirement: Local knowledge ingestion
The system SHALL ingest `.txt`, `.md`, and `.pdf` knowledge sources from local files and directories, split them into deterministic chunks, generate embeddings, and persist chunk content plus citation metadata into embedded storage.

#### Scenario: Ingest supported file and directory sources
- **WHEN** a caller passes a supported local file or directory source to `AddKnowledge`
- **THEN** the library loads each `.txt`, `.md`, and `.pdf` file, chunks its content, embeds the chunks, and stores them with source path, title, chunk ID, and offset metadata

#### Scenario: Reject unsupported knowledge source types
- **WHEN** a caller passes a source that is not `.txt`, `.md`, or `.pdf`
- **THEN** the library MUST return an unsupported-source error

#### Scenario: Load text from a local PDF
- **WHEN** a caller ingests a supported local PDF file containing textual page content
- **THEN** the library MUST extract the page text and pass that text through the normal chunking and embedding pipeline
