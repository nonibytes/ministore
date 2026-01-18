# Ministore Load Tests

Needle-in-a-haystack benchmarks for measuring search performance at scale.

## Usage

```bash
# Run 100k document test (builds binary automatically)
./load/load_test.sh 100k

# Run 1M document test
./load/load_test.sh 1m

# Run both
./load/load_test.sh both

# Clean up generated data
./load/load_test.sh clean
```

## What It Tests

1. **FTS needle search** - Find a unique term in FTS index
2. **Keyword exact match** - Find document by unique category
3. **Number range search** - Find document by priority > 900
4. **Complex query** - FTS term + keyword filter
5. **Broad search** - Return 100 results from ~10% of corpus

## Generated Data

- `data/100k.jsonl` - 100,000 document corpus (~15MB)
- `data/1m.jsonl` - 1,000,000 document corpus (~150MB)
- `data/*.db` - SQLite indexes

The "needle" document has:
- `title: "NEEDLE_UNIQUE_XYZ_12345"`
- `body: "...MAGIC_HAYSTACK_FINDER..."`
- `category: "needle"`
- `priority: 999`

## Benchmark Results (100k documents)

Tested on Linux, Intel CPU. Results are averages over 10 iterations.

| Test | Go (pure) | Go (CGO) | Rust |
|------|-----------|----------|------|
| FTS needle | 19ms | 17ms | 12ms |
| Keyword exact | 18ms | 15ms | 11ms |
| Number range | 20ms | 17ms | 13ms |
| Complex query | 19ms | 18ms | 13ms |
| Broad (100 results) | 50ms | 50ms | 53ms |

### Key Observations

- **Pure Go driver** (default): ~1.5-2x slower than Rust for simple queries
- **CGO driver**: ~1.3-1.4x slower than Rust for simple queries
- **Broad queries**: Go matches or beats Rust when returning many results
- Index format is **fully compatible** between Go and Rust versions

### Why Pure Go is Default

The pure Go SQLite driver (`modernc.org/sqlite`) is used by default because:
- No C compiler required
- Cross-compilation works out of the box
- Portable across all Go-supported platforms
- Performance is acceptable for most use cases

## Building with CGO Driver

For maximum performance, build with the CGO SQLite driver:

```bash
# Install CGO driver
go get github.com/mattn/go-sqlite3

# Build with CGO (requires C compiler and SQLite dev headers)
CGO_ENABLED=1 go build -tags "cgo_sqlite fts5" -o bin/ministore-cgo ./cmd/ministore
```

Requirements:
- C compiler (gcc/clang)
- SQLite development headers (`libsqlite3-dev` on Debian/Ubuntu)

## Reproducing Benchmarks

```bash
# 1. Generate corpus and run Go benchmark
./load/load_test.sh 100k

# 2. For CGO comparison, build CGO binary first
CGO_ENABLED=1 go build -tags "cgo_sqlite fts5" -o bin/ministore-cgo ./cmd/ministore

# 3. Run manual benchmark
DB=load/data/100k.db
CLI=bin/ministore  # or bin/ministore-cgo

for i in {1..3}; do $CLI search -i "$DB" -w "category:cat5" --limit 1 > /dev/null; done

for query in "NEEDLE_UNIQUE_XYZ_12345" "category:needle" "priority>900"; do
    total=0
    for i in {1..10}; do
        start=$(date +%s.%N)
        $CLI search -i "$DB" -w "$query" --limit 5 > /dev/null 2>&1
        end=$(date +%s.%N)
        elapsed=$(echo "($end - $start) * 1000" | bc)
        total=$(echo "$total + $elapsed" | bc)
    done
    echo "$query: $(echo "scale=3; $total / 10" | bc)ms"
done
```

## Cross-Version Compatibility

The SQLite index format is identical between Go and Rust implementations:

```bash
# Go can read Rust-created indexes
./bin/ministore search -i /path/to/rust/index.db -w "category:needle"

# Rust can read Go-created indexes
ministore search -i /path/to/go/index.db -w "category:needle"
```
