# OKF v0.2 implementation roadmap

## Repositories

- Go: `codex/okf-implementation`; latest feature commit
  `83716fb1c1596c51b44f7d8b705f61218d96904a`
- Rust: `codex/okf-implementation`; latest feature commit
  `48965bd43cc901e1abeabde77e00729d765cc9ad`

## Work queue

### Phase 1 — Shared contract and fixtures

- [ ] Add the pinned fixture corpus and matching manifests.
- [ ] Freeze finding codes and normalized validation, projection, and sync outputs.
- [ ] Implement lossless delimiter parsing and YAML accessors in both languages.
- [ ] Add parser properties, fuzz targets, and raw-byte tests.

### Phase 2 — MiniStore streaming primitives

- [x] Implement and test ordered streaming path scans in Go SQLite, Go PostgreSQL,
  and Rust SQLite.
- [ ] Implement transaction-scoped streamed writers in Go and Rust.
- [ ] Delegate existing in-memory batches to the streamed writers.
- [ ] Test rollback, cancellation, mixed operations, document frequencies, ordering,
  and Rust connection locking.

### Phase 3 — Parsing and validation

- [ ] Add the Go `okf` package and Rust `ministore-okf` crate.
- [ ] Implement the complete finding catalog and deterministic disk-backed reports.
- [ ] Validate reserved files, versions, optional families, legacy metadata, actors,
  lifecycle, provenance, trust, and attested computations.
- [ ] Add equivalent `okf validate` library and CLI behavior.

### Phase 4 — Graph and projection

- [ ] Implement CommonMark link extraction and safe local target resolution.
- [ ] Implement disk-backed edges, forward links, and backlinks.
- [ ] Implement the canonical schema, projection, and hash.
- [ ] Make Go and Rust projection fixtures byte-identical.

### Phase 5 — Synchronization

- [ ] Implement secure temporary SQLite staging and cleanup.
- [ ] Implement disk-backed target comparison and action classification.
- [ ] Implement atomic streamed apply, dry run, strict mode, schema verification,
  missing-index creation, reports, and unchanged timestamp preservation.
- [ ] Add equivalent `okf sync` CLI behavior and backend tests.

### Phase 6 — Parity, performance, and release quality

- [ ] Add the top-level `scripts/verify-okf.sh` gate and PostgreSQL container test.
- [ ] Prove cross-language fixture and CLI parity.
- [ ] Prove aggregate bundle size does not drive application RAM.
- [ ] Cover disk-full, malformed/adversarial input, rollback, and exact retrieval.
- [ ] Complete user and operational documentation.
- [ ] Run authenticated read-only `claude-yolo` review and resolve findings.

## Verification record

- `go test ./...` — passed.
- `cargo test --workspace` — passed (54 unit tests and 17 integration tests,
  including both ordered path-scan tests).
- `MINISTORE_POSTGRES_TEST_DSN=... go test ./ministore -run
  '^TestScanPathsPostgres$' -count=1` against `postgres:17-alpine` — passed.
- `git diff --check` in both repositories — passed before commit.

## Blockers

- None.

## Learnings

- The Go storage adapters centralize administrative SQL in `storage.SQL`; bytewise
  PostgreSQL ordering requires `COLLATE "C"`.
- Rust scans must hold the connection mutex while yielding SQLite rows, so callbacks
  must not recursively call the same `Index`.
