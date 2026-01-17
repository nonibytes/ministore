# ministore

This is a skeleton codebase for a multi-backend Go implementation of `ministore`.

Original Rust-based library: https://github.com/nonibyte/ministore-rust
(need to checkout with gh cli)

- Library entrypoint: `pkg/ministore.Open`
- CLI entrypoint: `cmd/ministore`

Replace the module path in `go.mod` with your own.

## Build

```bash
go build ./...
```

## Run (CLI skeleton)

```bash
go run ./cmd/ministore --help
```
