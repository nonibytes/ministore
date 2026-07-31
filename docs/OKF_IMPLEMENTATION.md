# OKF v0.2 implementation roadmap

## Repositories

- Go: `codex/okf-implementation`; latest feature commit
  `5d4f33fbe5abc262168da4f6abe280f3d6253a2c`
- Rust: `codex/okf-implementation`; latest feature commit
  `a5ca1726ca7cebe67572b0efa3a657e144f521a0`

## Work queue

### Phase 1 — Shared contract and fixtures

- [x] Add the pinned fixture corpus and matching manifests.
- [x] Freeze the finding-code catalog in both public APIs.
- [ ] Add normalized validation, projection, graph, hash, and sync golden outputs.
  (in progress: shared base-conformance validation JSONL complete)
- [x] Implement lossless delimiter parsing and YAML accessors in both languages.
- [x] Add parser property/fuzz coverage and raw-byte tests.

### Phase 2 — MiniStore streaming primitives

- [x] Implement and test ordered streaming path scans in Go SQLite, Go PostgreSQL,
  and Rust SQLite.
- [x] Implement transaction-scoped streamed writers in Go and Rust.
- [x] Delegate existing in-memory batches to the streamed writers.
- [x] Test rollback, cancellation, mixed operations, document frequencies, ordering,
  and Rust connection locking.

### Phase 3 — Parsing and validation

- [ ] Add the Go `okf` package and Rust `ministore-okf` crate. (in progress:
  lossless parser, YAML-node access, base validator, and reserved-file validator
  complete)
- [ ] Implement the complete finding catalog and deterministic disk-backed reports.
  (in progress: catalog frozen; private SQLite entry/finding stage and ordered
  finding stream complete)
- [ ] Validate reserved files, versions, optional families, legacy metadata, actors,
  lifecycle, provenance, trust, and attested computations. (base concept and
  AST-aware reserved-file conformance complete)
- [ ] Add equivalent `okf validate` library and CLI behavior.

### Phase 4 — Graph and projection

- [ ] Implement CommonMark link extraction and safe local target resolution.
- [ ] Implement disk-backed edges, forward links, and backlinks.
- [ ] Implement the canonical schema, projection, and hash.
- [ ] Make Go and Rust projection fixtures byte-identical.

### Phase 5 — Synchronization

- [ ] Implement secure temporary SQLite staging and cleanup. (validation entry and
  finding tables, owner-only lifecycle, and cleanup complete; graph/action tables
  pending)
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
- `cargo test --workspace` — passed (54 core unit tests, 22 core integration
  tests, and 18 OKF parser/validation tests).
- `git diff --check` in both repositories — passed before commit.
- `sha256sum -c testdata/okf/v0.2/MANIFEST.sha256` in both repositories —
  passed for all 110 fixture and golden files.
- `diff -qr` between the Go and Rust fixture trees — no differences.
- `diff -qr` between each committed upstream sample and commit
  `3fcbb9f828c2f23d109c855ee403c3a4c81f3a96` — no differences; pinned
  `SPEC.md` SHA-256 also matched the design.
- `go test -mod=readonly ./okf` and `go vet ./okf` — passed.
- `go test ./okf -run '^$' -fuzz '^FuzzParseDocument$' -fuzztime=5s` —
  passed after 213,163 executions.
- `CGO_ENABLED=0 go test -mod=readonly ./...` — passed.
- `go test -race ./ministore/...` and `go vet ./...` — passed.
- `MINISTORE_POSTGRES_TEST_DSN=... go test ./ministore -run
  'Test(WriteBatch|ScanPaths)Postgres' -count=1` against `postgres:17-alpine` —
  passed, including rollback and keyword document-frequency checks.
- `rustfmt --check` on the new `ministore-okf` crate — passed.
- `cargo clippy -p ministore-okf --all-targets -- -D warnings` — passed.
- `cargo test -p ministore-okf` — passed, including generated arbitrary-input
  property cases; `cargo test --workspace` also passed.
- The shared `expected/base-validation.jsonl` golden passed byte-for-byte in Go
  and Rust; the complete fixture trees and manifests remain identical.

## Blockers

- None.

## Learnings

- The Go storage adapters centralize administrative SQL in `storage.SQL`; bytewise
  PostgreSQL ordering requires `COLLATE "C"`.
- Rust scans must hold the connection mutex while yielding SQLite rows, so callbacks
  must not recursively call the same `Index`.
- The offline corpus contains synthetic contract fixtures plus exact GA4, Stack
  Overflow, Bitcoin, and Acme Retail snapshots; expected normalized artifacts are
  intentionally generated only after the parser and validation contracts exist.
- The design's delimiter prose mentions stale example code `OKF010`, and its sample
  JSON uses stale `OKF241`; both implementations follow the authoritative catalog:
  BOM is `OKF107` and missing local targets are `OKF400`.
- Rust uses the low-level `yaml-rust2` event API because its convenience loader
  rejects duplicate keys. Alias nodes remain finite and resolve lazily, so cyclic
  extensions are preserved without whole-tree expansion.
- Go streamed puts release keyword-ID and document-frequency state after each
  document. This keeps memory proportional to one input document without an input
  limit; optional byte-bounded coalescing remains a measured optimization, not a
  correctness requirement. Deletes flush pending deltas before decrementing.
- PostgreSQL treats a missing FTS table as an error that aborts the transaction even
  if the adapter later ignores the error. Schema-aware deletes therefore skip FTS
  SQL entirely when an index has no text fields.
- Bundle enumeration writes paths directly to SQLite from streaming directory
  iterators. SQLite supplies global bytewise ordering, avoiding a complete in-memory
  path list even for a flat directory containing the whole bundle.
- Parser-specific YAML syntax locations are omitted only from normalized parity
  goldens; stable finding code, severity, path, and specification section remain
  identical, while each public finding retains any location its parser provides.
- Reserved Markdown is parsed one file at a time with CommonMark AST/event APIs.
  The validators ignore markup inside fenced code, accept inline and reference
  links, report source-line starts consistently across languages, and stage all
  findings in SQLite before deterministic emission.
