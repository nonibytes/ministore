# OKF v0.2 implementation roadmap

## Repositories

- Go branch: `codex/okf-implementation`; feature commit `5700be2`
- Rust branch: `codex/okf-implementation`; feature commit `251c942`

## Phase checklist

### Phase 1 — Shared contract and fixtures

- [x] Pinned v0.2 corpus and identical manifests in both repositories.
- [x] Stable finding catalog and deterministic normalized validation goldens.
- [x] Shared canonical projection and hash golden.
- [x] Cross-language fixture, validation, graph, projection, hash, schema, and sync comparison in `scripts/verify-okf.sh`.

### Phase 2 — MiniStore streaming primitives

- [x] Ordered bytewise path scans for Go SQLite/PostgreSQL and Rust SQLite.
- [x] Transaction-scoped streamed writers with in-memory batch delegation.
- [x] Rollback, cancellation, mixed-operation, document-frequency, ordering, connection-locking, and large-stream tests.

### Phase 3 — Parsing and validation

- [x] Lossless one-document parsers and typed YAML-node accessors in Go and Rust.
- [x] Disk-backed deterministic findings and complete stable finding-code use.
- [x] Reserved files, versions, lifecycle, tags, legacy compatibility, provenance, actors, trust, footnotes, and attested-computation validation.
- [x] Unicode case-fold collision parity test.
- [x] Equivalent streaming `okf validate` libraries and CLIs with pretty/JSON and strict behavior.

### Phase 4 — Graph and projection

- [x] CommonMark link extraction excluding images and code.
- [x] Safe URI-reference resolution, one-time segment decoding, root-escape checks, and broken-link findings.
- [x] Disk-backed deduplicated edges with per-concept forward links and backlinks.
- [x] Fixed schema, one-document projection, raw-source retention, canonical JSON, and SHA-256 hash.
- [x] Byte-identical Go/Rust projections and hashes, including integer, large, and small-exponent usage counts.

### Phase 5 — Synchronization

- [x] Owner-only temporary SQLite stages with normal/handled-error cleanup.
- [x] Disk-backed existing paths and add/update/unchanged/delete actions.
- [x] Schema verification, strict and dry-run blocking, missing-index creation, unchanged timestamp preservation, and reports.
- [x] One streamed atomic target transaction with no nested public writes.
- [x] Go SQLite/PostgreSQL and Rust SQLite support; PostgreSQL sync integration test.
- [x] Equivalent `okf sync` CLIs and safe missing/wrong-index handling.

### Phase 6 — Parity, performance, and release quality

- [x] Offline end-to-end verification driver with fixture manifests and live CLI/database parity.
- [x] Aggregate-bundle memory regression check and large streamed transaction coverage.
- [x] Malformed/adversarial parser properties, exact raw retrieval, validation/dry-run no-write, rollback, and schema mismatch coverage.
- [x] User and operational documentation for temporary storage, sensitive source, queries, rebuilds, and PostgreSQL WAL/serialization.
- [x] Authenticated read-only `claude-yolo` review run with `ANTHROPIC_API_KEY` unset; actionable PostgreSQL target detection and numeric canonicalization findings fixed and regression-tested.

## Verification record

- `scripts/verify-okf.sh` — passed after final fixes. It ran fixture manifest checks; Go vet, pure-Go tests, race tests, CGO/FTS5 tests; strict Rust workspace Clippy and all-feature tests; live normalized validation/sync parity; byte-identical database projections/hashes; dry-run missing-target safety; memory scaling; and isolated PostgreSQL core/sync tests.
- `go test ./okf` — passed, including sync consistency, projection golden, Unicode case folding, numeric exponent hashing, and malformed-input coverage.
- `cargo test -p ministore-okf` — passed, including the corresponding parity and sync tests.
- `git diff --check` — passed in both repositories before feature commits.
- `env -u ANTHROPIC_API_KEY claude-yolo ...` — completed read-only review. Findings requiring changes were addressed; long transactions and unsupported concurrent sync remain explicitly documented design semantics, and killed-process orphan cleanup remains delegated to operating-system temporary-directory policy.
- The requested `cargo fmt --all -- --check` is not a usable repository-wide gate on the inherited Rust baseline: it reports 4,786 lines of pre-existing formatting differences across unrelated core and CLI files. The verifier instead runs `rustfmt --check` on every changed OKF Rust file. Strict workspace Clippy and all workspace tests pass. Formatting the entire legacy repository would be an unrelated 47-file rewrite, so it was not included.

## Blockers

- None.

## Implementation learnings

- SQLite staging owns complete path, source, finding, graph, and action state; application memory holds one source/projection and its adjacency lists.
- Link candidates and actions use keyset iteration so no complete graph or operation collection is materialized in RAM.
- CLI JSON validation findings spool to an owner-private temporary file instead of accumulating a finding array.
- PostgreSQL schema emptiness is checked without creating it, so connectivity or invalid-index errors cannot be mistaken for a missing target.
- Numeric projection values require explicit cross-language exponent normalization; golden and edge-case tests lock the hash contract.
- Go `cases.Fold` and Rust `unicode-casefold` agree on the pinned non-ASCII collision regression; both stage collision checks with identical query behavior.
