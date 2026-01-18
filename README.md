# Ministore

A lightweight, embedded document search engine built on SQLite and FTS5. Ministore provides full-text search, structured filtering, and rich query capabilities in a single-file database.

This is the Go implementation. See also: [Rust implementation](https://github.com/nonibytes/ministore-rust)

## Features

- **Full-Text Search**: Powered by SQLite's FTS5 for fast, relevant text search
- **Structured Data**: Support for keyword, number, date, and boolean fields
- **Rich Query Language**: Combine text search with structured filters using an intuitive query syntax
- **Flexible Ranking**: BM25 scoring with customizable field weights and boost expressions
- **Cursor-Based Pagination**: Efficient pagination for large result sets
- **Schema Management**: Define schemas with multiple field types and multi-value support
- **Batch Operations**: Transactional batch inserts and deletes
- **Multi-Backend**: SQLite (default) and PostgreSQL support
- **Zero CGO by Default**: Pure Go SQLite driver for easy cross-compilation
- **CLI & Library**: Use as a Go library or standalone command-line tool

## Installation

### As a Library

```bash
go get github.com/ministore/ministore
```

### CLI Tool

```bash
# Build from source
git clone https://github.com/nonibytes/ministore.git
cd ministore
go build -o bin/ministore ./cmd/ministore

# Or install directly
go install github.com/ministore/ministore/cmd/ministore@latest
```

## Quick Start

### Using the CLI

```bash
# Create an index with a schema
ministore index create -i docs.db --schema schema.json

# Add documents
ministore put -i docs.db --path /blog/hello-world \
  --set title="Hello World" \
  --set tags=rust,tutorial \
  --set published=2024-01-15

# Search
ministore search -i docs.db -w "hello AND tags:rust"

# Import from JSONL (stdin)
cat documents.jsonl | ministore put -i docs.db --json
```

### Using the Library

```go
package main

import (
    "context"
    "fmt"
    "github.com/ministore/ministore/ministore"
    "github.com/ministore/ministore/ministore/storage/sqlite"
)

func main() {
    ctx := context.Background()

    // Create schema
    schema := ministore.NewSchema()
    schema.AddField("title", ministore.FieldSpec{Type: ministore.FieldText, Weight: 2.0})
    schema.AddField("body", ministore.FieldSpec{Type: ministore.FieldText, Weight: 1.0})
    schema.AddField("tags", ministore.FieldSpec{Type: ministore.FieldKeyword, Multi: true})
    schema.AddField("published", ministore.FieldSpec{Type: ministore.FieldDate})

    // Create index
    adapter := sqlite.New("docs.db")
    ix, err := ministore.Create(ctx, adapter, schema, ministore.DefaultIndexOptions())
    if err != nil {
        panic(err)
    }
    defer ix.Close()

    // Insert a document
    err = ix.PutJSON(ctx, []byte(`{
        "path": "/blog/hello-world",
        "title": "Hello World",
        "body": "Welcome to my blog about Go programming",
        "tags": ["go", "tutorial"],
        "published": "2024-01-15T10:00:00Z"
    }`))
    if err != nil {
        panic(err)
    }

    // Search
    results, err := ix.Search(ctx, "go programming", ministore.SearchOptions{Limit: 10})
    if err != nil {
        panic(err)
    }

    for _, item := range results.Items {
        fmt.Printf("%s: %s\n", item.Path, item.Data["title"])
    }
}
```

## Query Language

Ministore supports a powerful query syntax for combining full-text search with structured filters:

### Text Search

```
hello world              # Match documents containing both words
"hello world"            # Exact phrase match
hello OR world           # Match either word
hello NOT world          # Match hello but not world
```

### Field Filters

```
tags:rust                # Keyword field exact match
published:>2024-01-01    # Date comparison
views:>=1000             # Numeric comparison
featured:true            # Boolean field
```

### Combined Queries

```
"rust tutorial" AND tags:beginner AND published:>2024-01-01
machine learning AND (tags:python OR tags:tensorflow)
NOT archived:true
```

### Operators

- **Text**: `AND`, `OR`, `NOT`, `"phrase"`
- **Numeric/Date**: `>`, `>=`, `<`, `<=`, `=`
- **Boolean**: `true`, `false`

## CLI Reference

### Index Management

```bash
# Create index with schema file
ministore index create -i myindex.db --schema schema.json

# View schema
ministore index schema -i myindex.db

# Optimize index (FTS5 optimize + VACUUM)
ministore index optimize -i myindex.db
```

### Document Operations

```bash
# Insert single document
ministore put -i myindex.db --path /doc/1 \
  --set field1=value1 --set field2=value2

# Insert from JSONL (stdin)
echo '{"path": "/doc/1", "title": "Hello"}' | ministore put -i myindex.db --json

# Bulk import from file
cat documents.jsonl | ministore put -i myindex.db --json

# Get document
ministore get -i myindex.db --path /doc/1

# Peek at metadata
ministore peek -i myindex.db --path /doc/1

# Delete by path
ministore delete -i myindex.db --path /doc/1

# Delete by query
ministore delete -i myindex.db -w "archived:true"
```

### Search

```bash
# Basic search
ministore search -i myindex.db -w "query"

# With pagination
ministore search -i myindex.db -w "query" --limit 20

# Custom ranking
ministore search -i myindex.db -w "query" --rank "bm25 + boost"

# Select fields
ministore search -i myindex.db -w "query" --show "title,summary"

# Output formats
ministore search -i myindex.db -w "query" --format json
ministore search -i myindex.db -w "query" --format paths
ministore search -i myindex.db -w "query" --format pretty

# Explain query
ministore search -i myindex.db -w "query" --explain
```

### Discovery

```bash
# List all fields
ministore discover fields -i myindex.db

# Show top values for a field
ministore discover values -i myindex.db --field tags --limit 10

# Field statistics
ministore stats -i myindex.db --field views
ministore stats -i myindex.db --field views -w "published:>2024-01-01"
```

## Schema Definition

Define schemas via JSON file:

```json
{
  "fields": {
    "title": {
      "type": "text",
      "weight": 2.0
    },
    "body": {
      "type": "text",
      "weight": 1.0
    },
    "tags": {
      "type": "keyword",
      "multi": true
    },
    "published": {
      "type": "date"
    },
    "views": {
      "type": "number"
    },
    "featured": {
      "type": "bool"
    }
  }
}
```

### Field Types

- **text**: Full-text searchable content (FTS5 indexed)
- **keyword**: Exact-match strings (e.g., tags, categories)
- **number**: Numeric values for filtering and ranking
- **date**: ISO 8601 timestamps stored as Unix milliseconds
- **bool**: Boolean values (true/false)

## Backend Support

### SQLite (Default)

```bash
ministore index create -i myindex.db --schema schema.json
```

### PostgreSQL

```bash
ministore index create -i "postgres://user:pass@localhost/db" \
  --backend postgres \
  --schema schema.json \
  --schema-name ministore
```

## Performance

### Benchmark Results (100k documents)

| Operation | Go (pure) | Go (CGO) | Rust |
|-----------|-----------|----------|------|
| **Import** | 42s | 31s | 12s |
| FTS search | 19ms | 17ms | 12ms |
| Keyword exact | 18ms | 15ms | 11ms |
| Number range | 20ms | 17ms | 13ms |
| Complex query | 19ms | 18ms | 13ms |
| Broad (100 results) | 50ms | 50ms | 53ms |

### Building with CGO (Optional)

For maximum performance, build with the CGO SQLite driver:

```bash
CGO_ENABLED=1 go build -tags "cgo_sqlite fts5" -o bin/ministore ./cmd/ministore
```

Requirements: C compiler, SQLite dev headers (`libsqlite3-dev`)

### Why Pure Go is Default

The pure Go SQLite driver (`modernc.org/sqlite`) is used by default because:
- No C compiler required
- Cross-compilation works out of the box
- Portable across all Go-supported platforms
- Performance is acceptable for most use cases

## Cross-Compatibility

The SQLite index format is identical between Go and Rust implementations:

```bash
# Go can read Rust-created indexes
./bin/ministore search -i /path/to/rust/index.db -w "category:needle"

# Rust can read Go-created indexes
ministore search -i /path/to/go/index.db -w "category:needle"
```

## Project Structure

```
ministore/
├── cmd/ministore/       # CLI application
├── ministore/           # Core library
│   ├── storage/         # Backend adapters
│   │   ├── sqlite/      # SQLite adapter
│   │   └── postgres/    # PostgreSQL adapter
│   └── ops/             # Query operations
└── load/                # Benchmark tests
```

## Running Tests

```bash
# Unit tests
go test ./...

# Load tests
./load/load_test.sh 100k
```

## License

MIT License

## Links

- **Go Implementation**: https://github.com/nonibytes/ministore
- **Rust Implementation**: https://github.com/nonibytes/ministore-rust
