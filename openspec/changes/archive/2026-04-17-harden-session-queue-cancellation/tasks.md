## 1. Session Execution Gate

- [x] 1.1 Replace the blocking session execution mutex with a context-aware execution slot for queued sync and stream requests.
- [x] 1.2 Add regression tests for queued request cancellation while another same-session operation is in flight.

## 2. Verification

- [x] 2.1 Run tests, vet, and strict OpenSpec validation after the execution-gate change.
