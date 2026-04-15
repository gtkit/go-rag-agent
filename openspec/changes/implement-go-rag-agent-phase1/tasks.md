## 1. OpenSpec And Dependency Setup

- [x] 1.1 Add the initial Go module dependencies for Eino OpenAI-compatible model and embedding packages, chromem-go, gtkit/json, and gtkit/logger.
- [ ] 1.2 Create the root-package public contracts for config, errors, sources, answers, citations, callbacks, and stream events.

## 2. Knowledge Ingestion And Storage

- [ ] 2.1 Implement local file and directory source resolution for `.txt` and `.md` knowledge inputs.
- [ ] 2.2 Implement deterministic document loading, chunking, query normalization, follow-up rewriting, and bounded context assembly.
- [ ] 2.3 Implement the embedded chromem-backed vector store adapter and its tests.

## 3. Session Memory And Execution

- [ ] 3.1 Implement bounded session memory and telemetry dispatch helpers.
- [ ] 3.2 Implement OpenAI-compatible chat and embedding adapters using Eino-ext packages.
- [ ] 3.3 Implement the retrieval tool and Eino ReAct runner.
- [ ] 3.4 Implement agent construction, knowledge ingestion, synchronous session asking, and citation generation.
- [ ] 3.5 Implement callback-based streaming, same-session serialization, cancellation handling, and concurrency tests.

## 4. Documentation And Verification

- [ ] 4.1 Add README, examples, GoDoc example coverage, and benchmarks for chunking and retrieval paths.
- [ ] 4.2 Run lint, vet, race tests, benchmarks, and strict OpenSpec validation, then update task status to complete.
