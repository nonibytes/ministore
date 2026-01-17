# Ministore in Go — Multi-Backend Design (SQLite, Postgres, Redis)

This document specifies a Go implementation of `ministore` (library + CLI) with a **pluggable storage adapter** architecture. It is intentionally “close to code”: concrete packages, files, structs, function signatures, data layouts, and execution mechanics—**everything short of implementation**.

---

## 1. Goals and invariants

### 1.1 Must match v1 behavior (from Rust spec)

* Explicit schemas (types, `multi`, text weights).
* Single logical index per `--index` name.
* Item identity: **path** is unique, hierarchical.
* Query language: boolean (`& | ! ()`), fielded predicates, wildcards for keyword/path, numeric/date comparisons & ranges, `has:` existence, bare-text shorthand for FTS.
* Query guardrails:

  * Must have at least one positive anchor.
  * Prefix/contains minimum lengths and expansion caps.
* Ranking modes: `default`, `recency`, `field:<name>`, `none`.
* Cursor pagination with stable ordering and query/schema validation hash.

### 1.2 New requirement

* Pluggable storage adapters.
* Default adapters shipped:

  * **SQLite** (single-file, like Rust).
  * **Postgres** (shared DB; index as namespace).
  * **Redis** (aim for near-full feature parity).

---

## 2. Repository layout

```
/go.mod
/cmd/ministore/main.go

/internal/cli/
  root.go
  global.go
  output.go
  resolve.go
  commands/
    index.go
    put.go
    get.go
    peek.go
    delete.go
    search.go
    discover.go
    stats.go

/pkg/ministore/
  doc.go
  errors.go
  constants.go

  schema/
    schema.go
    validate.go
    json.go

  item/
    item.go

  query/
    ast.go
    lexer.go
    parser.go
    normalize.go

  plan/
    plan.go
    typecheck.go
    explain.go

  cursor/
    cursor.go
    hash.go

  index/
    index.go
    options.go
    batch.go

  backend/
    backend.go
    capabilities.go
    registry.go

  adapters/
    sqlite/
      sqlite.go
      ddl.go
      sql.go
      fts.go
      cursor_store.go
    postgres/
      postgres.go
      ddl.go
      sql.go
      fts.go
      cursor_store.go
      registry.go
    redis/
      redis.go
      schema_fts.go
      query_fts.go
      cursor_store.go
      lua.go
      registry.go

  internal/
    util/
      time.go
      glob.go
      jsoncoerce.go
      sqlident.go
      redisescape.go
```

---

## 3. Public Go library API

### 3.1 Core types (package `pkg/ministore/index`, `schema`, `item`)

**`schema.Schema`**

```go
type FieldType string
const (
  FieldKeyword FieldType = "keyword"
  FieldText    FieldType = "text"
  FieldNumber  FieldType = "number"
  FieldDate    FieldType = "date"
  FieldBool    FieldType = "bool"
)

type FieldSpec struct {
  Type   FieldType `json:"type"`
  Multi  bool      `json:"multi,omitempty"`
  Weight *float64  `json:"weight,omitempty"` // text only
}

type Schema struct {
  Fields map[string]FieldSpec `json:"fields"`
}
```

**`item.ItemView`**

```go
type ItemMeta struct {
  CreatedAtMs int64
  UpdatedAtMs int64
}

type ItemView struct {
  Path string
  Doc  map[string]any // original JSON object decoded
  Meta ItemMeta
}
```

**`index.Index`**

```go
type Index struct {
  name   string
  schema schema.Schema
  store  backend.IndexStore
  opts   IndexOptions
}

type IndexOptions struct {
  CursorTTL time.Duration // default 1h
}
```

**Search/Discover/Stats outputs**

```go
type RankMode struct {
  Kind  string // "default" | "recency" | "field" | "none"
  Field string // for Kind=="field"
}

type CursorMode string
const (
  CursorShort CursorMode = "short"
  CursorFull  CursorMode = "full"
)

type OutputFieldSelector struct {
  Mode   string   // "none" | "all" | "fields"
  Fields []string // if Mode=="fields"
}

type SearchOptions struct {
  Rank       RankMode
  Limit      int
  After      string // cursor token
  CursorMode CursorMode
  Show       OutputFieldSelector
  Explain    bool
}

type SearchResultPage struct {
  Items        []map[string]any
  NextCursor   string // empty if none
  HasMore      bool
  ExplainPlan  []string // semantic steps
  ExplainQuery string   // backend SQL/FT query (if Explain)
}
```

### 3.2 Opening/creating indexes (multi-backend)

We expose a backend-agnostic entrypoint:

```go
type OpenOptions struct {
  Backend string // "sqlite" | "postgres" | "redis"
  // Backend-specific connection info:
  SQLitePath string // directory or explicit file path
  PostgresDSN string
  RedisAddr string
  RedisPassword string
  RedisDB int
}

func Open(ctx context.Context, opts OpenOptions) (*backend.Client, error)
```

Then:

```go
type Client struct {
  backend backend.Backend
}

func (c *Client) CreateIndex(ctx context.Context, name string, schema schema.Schema, idxOpts IndexOptions) (*index.Index, error)
func (c *Client) OpenIndex(ctx context.Context, name string, idxOpts IndexOptions) (*index.Index, error)
func (c *Client) ListIndexes(ctx context.Context) ([]backend.IndexInfo, error)
func (c *Client) DropIndex(ctx context.Context, name string) error
```

---

## 4. Backend abstraction (package `backend`)

### 4.1 Interfaces

```go
type Backend interface {
  Capabilities() Capabilities
  Registry() Registry // list/create/drop schema metadata
  OpenIndexStore(ctx context.Context, indexName string, sch schema.Schema, opts index.IndexOptions) (IndexStore, error)
}

type Registry interface {
  Create(ctx context.Context, name string, sch schema.Schema) error
  Get(ctx context.Context, name string) (schema.Schema, error)
  List(ctx context.Context) ([]IndexInfo, error)
  Drop(ctx context.Context, name string) error
}

type IndexStore interface {
  // CRUD
  Put(ctx context.Context, doc map[string]any) error
  Get(ctx context.Context, path string) (item.ItemView, error)
  Peek(ctx context.Context, path string) (map[string]any, error)
  Delete(ctx context.Context, path string) (bool, error)
  DeleteWhere(ctx context.Context, compiled plan.CompiledQuery) (int, error)

  // Query
  Search(ctx context.Context, compiled plan.CompiledQuery, opts index.SearchOptions) (index.SearchResultPage, error)

  // Explore
  DiscoverValues(ctx context.Context, field string, compiledOrNil *plan.CompiledQuery, top int) ([]backend.ValueCount, error)
  DiscoverFields(ctx context.Context) ([]backend.FieldOverviewRow, error)
  Stats(ctx context.Context, field string, compiledOrNil *plan.CompiledQuery) (map[string]any, error)

  // Schema ops
  ApplySchemaAdditive(ctx context.Context, newSchema schema.Schema) error
  MigrateRebuild(ctx context.Context, newName string, newSchema schema.Schema) error

  // Maintenance
  Optimize(ctx context.Context) error
}
```

### 4.2 Capabilities model

```go
type Capabilities struct {
  // Query feature support
  FullText bool
  KeywordExact bool
  KeywordPrefix bool
  KeywordContains bool
  KeywordGlob bool
  PathPrefix bool
  PathGlob bool
  HasPredicate bool
  NumberOps bool
  DateOps bool
  BoolOps bool

  // Ranking support
  RankDefault bool
  RankRecency bool
  RankField bool
  RankNone bool

  // Pagination support
  CursorShort bool
  CursorFull bool

  // Discover/stats support
  DiscoverValues bool
  StatsAggregations bool
}
```

Adapters must set these accurately; planner rejects unsupported constructs early when possible.

---

## 5. Shared query pipeline (parse → normalize → compile)

This part is **backend-independent** and identical across adapters.

### 5.1 Query AST (package `query`)

Direct port of Rust concepts:

```go
type ExprKind int
const (
  ExprAnd ExprKind = iota
  ExprOr
  ExprNot
  ExprPred
)

type Expr struct {
  Kind ExprKind
  Left, Right *Expr
  Pred *Predicate
}

type PredicateKind int
const (
  PredHas PredicateKind = iota
  PredPathGlob
  PredKeyword
  PredText
  PredNumberCmp
  PredNumberRange
  PredDateAbsCmp
  PredDateAbsRange
  PredDateRelCmp
  PredBool
)

type KeywordPatternKind int
const (
  KwExact KeywordPatternKind = iota
  KwPrefix
  KwContains
  KwGlob
)

type CmpOp int
const (Eq Gt Gte Lt Lte CmpOp = ...)

type RelUnit int
const (H D W M Y RelUnit = ...)

type Predicate struct {
  Kind PredicateKind

  Field string
  Pattern string
  KwKind KeywordPatternKind

  TextField *string // nil for bare-text across all text fields
  FTS string

  Op CmpOp
  Num float64
  Lo, Hi float64

  EpochMs int64
  LoMs, HiMs int64
  Amount int64
  Unit RelUnit

  Bool bool
}
```

### 5.2 Normalization / guardrails (package `query/normalize.go` + `constants.go`)

Constants:

```go
const MinContainsLen = 3
const MinPrefixLen = 2
const MaxPrefixExpansion = 20000
```

Normalization rules:

* Must have a positive anchor:

  * Any `Text` predicate
  * Any exact keyword match
  * Path prefix with literal prefix before wildcard
  * Any numeric/date predicate
* Validate keyword patterns:

  * `prefix` must have literal length ≥ MinPrefixLen
  * `contains` inner length ≥ MinContainsLen
  * `glob` must have a literal prefix and ≥ MinPrefixLen

### 5.3 Typed compilation (package `plan`)

`plan.CompiledQuery` is backend-neutral:

```go
type CompiledQuery struct {
  Expr query.Expr           // normalized AST
  Schema schema.Schema      // snapshot used for hash + validation
  NowMs int64               // compile-time now
  ExplainSteps []string     // semantic steps
  RequiresText bool         // any text predicate present
}
```

The compile step:

1. Validates unknown fields.
2. Resolves `field:<value>` against schema:

   * If field is `text`, turns it into `PredText` (FTS) rather than keyword.
   * If field is `date` and value is exact ISO date, converts to absolute epoch predicate.
   * If field is `bool` and value is true/false, converts to bool predicate.
3. Resolves implicit fields `created`/`updated` (always available) as date predicates.

---

## 6. Cursor model (package `cursor`)

### 6.1 Cursor payload (shared semantics)

```go
type CursorPayloadKind string
const (
  CursorFts     CursorPayloadKind = "fts"     // score desc, item_id asc
  CursorRecency CursorPayloadKind = "recency" // updated desc, path asc
  CursorField   CursorPayloadKind = "field"   // rank desc, updated desc, path asc
  CursorNone    CursorPayloadKind = "none"    // item_id asc
  CursorRedis   CursorPayloadKind = "redis"   // adapter-specific, see Redis section
)

type CursorPayload struct {
  Kind CursorPayloadKind

  // FTS
  Score float64
  ItemID int64

  // Recency
  UpdatedAtMs int64
  Path string

  // Field
  Field string
  RankValue float64

  // Redis cursor API
  RedisCursorID int64
  RedisIndexName string
}

type CursorPosition struct {
  Payload CursorPayload
  Hash string // sha256(schema_json + "\n" + query + "\n" + rankMode)
}
```

### 6.2 Full cursor encoding

* JSON serialize `CursorPosition` then base64 URL-safe no pad.
* `cursor.DecodeFull(token)` returns `CursorPosition`.

### 6.3 Short cursors

* `c:<handle>` where handle is 96-bit random hex.
* Stored in backend cursor store with TTL.

  * SQL backends: `cursor_store` table.
  * Redis backend: Redis key with `EXPIRE`.

---

## 7. SQL adapters (SQLite + Postgres)

We keep SQL adapters as close as possible to Rust CTE-set-algebra execution.

### 7.1 Shared SQL generation strategy

Both SQL adapters implement:

* `compilePredToCTE(pred) -> (cteSQL, args, explainStep)`
* boolean combine:

  * AND → `INTERSECT`
  * OR → `UNION`
  * NOT → `EXCEPT`
* final select:

  * join items + result set
  * compute score column (always output 6th column)
  * apply cursor “after filter” in an outer query (so `score` is usable)
  * order + limit+1

We define a dialect interface in `plan/sqlgen` used by both adapters:

```go
type SQLDialect interface {
  Placeholder(n int) string // sqlite: "?" or "?1"; postgres: "$1"
  QuoteIdent(s string) string
  PathGlobOp() string // sqlite "GLOB", postgres "LIKE"/regex mapping
  SupportsFTS() bool
}
```

---

## 8. SQLite adapter (package `adapters/sqlite`)

### 8.1 Storage layout

**One index = one `.db` file**.

Tables (same as Rust):

* `meta(key,value)` stores schema_json, magic, version.
* `items(id INTEGER PK, path UNIQUE, data_json TEXT, created_at, updated_at)`
* `field_present(item_id, field)`
* `kw_dict(id, field, value, doc_freq)`
* `kw_postings(field, value_id, item_id)`
* `field_number(item_id, field, value REAL)`
* `field_date(item_id, field, value INTEGER)`
* `field_bool(item_id, field, value INTEGER)`
* `cursor_store(handle, payload, created_at, expires_at)`
* `search` FTS5 virtual table with one column per text field.

FTS5 behavior and multi-column field scoping matches the Rust rationale.

### 8.2 Key files and responsibilities

* `sqlite/sqlite.go`

  * `NewBackend(opts)`, `OpenIndexStore`
  * connection pool (simple `*sql.DB`)
  * pragmas WAL/NORMAL/foreign_keys
* `sqlite/ddl.go`

  * base DDL + FTS DDL generation from schema
* `sqlite/sql.go`

  * SQL constants and builders
* `sqlite/fts.go`

  * create/verify FTS columns, apply additive schema changes
* `sqlite/cursor_store.go`

  * implement short cursor persistence in `cursor_store`

### 8.3 Put mechanics (SQLite)

`Put` is a transaction:

1. Parse doc, validate schema, extract:

   * `path`, full JSON string, text columns, keyword values, numeric values, date epoch values, bool, present fields.
2. Upsert `items`:

   * if exists: update `data_json`, `updated_at`, preserve `created_at`
   * else insert with both timestamps
3. Load old keyword `value_id`s from `kw_postings` to update doc_freq correctly.
4. Delete old rows from postings/typed tables/present/fts row.
5. Insert present fields.
6. Insert keyword dict entries, postings; increment/decrement doc_freq.
7. Insert numbers/dates/bools.
8. Insert FTS row into `search` if schema has text fields.

### 8.4 Search mechanics (SQLite)

* Use compiled CTE SQL.
* Default rank:

  * if query includes any FTS predicate: join `search` and compute `(-bm25(search, weights...))` (then order desc).
  * otherwise: fall back to recency ordering.

Cursor “after filter” is injected in outer query exactly like Rust.

---

## 9. Postgres adapter (package `adapters/postgres`)

### 9.1 Index = schema namespace

Postgres is shared; we model “index” as a dedicated schema:

* Schema name: `ms_<index>` where `<index>` is sanitized `[a-z0-9_]+` with collisions rejected.

A global registry schema stores index metadata:

* Schema: `ministore_meta`
* Table: `ministore_meta.indexes(name TEXT PK, schema_json JSONB, created_at BIGINT)`

### 9.2 Tables per index schema

Same logical tables as SQLite, with types:

* `items(id BIGSERIAL PK, path TEXT UNIQUE, data_json JSONB, created_at BIGINT, updated_at BIGINT)`
* `field_present(item_id BIGINT, field TEXT, PRIMARY KEY(item_id,field))`
* `kw_dict(id BIGSERIAL PK, field TEXT, value TEXT, doc_freq BIGINT, UNIQUE(field,value))`
* `kw_postings(field TEXT, value_id BIGINT FK, item_id BIGINT FK, PRIMARY KEY(value_id,item_id))`
* `field_number(... value DOUBLE PRECISION)`
* `field_date(... value BIGINT)`
* `field_bool(... value BOOLEAN)`
* `cursor_store(handle TEXT PK, payload JSONB, created_at BIGINT, expires_at BIGINT)`

Indexes:

* btree on `(field,value)` for dict lookup
* btree on `(field,value)` for number/date lookups
* btree on `items(path)`, `items(updated_at)`, `items(created_at)`

**Contains wildcard performance**:

* Enable trigram for faster `LIKE '%foo%'` on dictionaries:

  * `CREATE EXTENSION IF NOT EXISTS pg_trgm;`
  * `CREATE INDEX ... ON kw_dict USING GIN (value gin_trgm_ops);`
    This supports fast dictionary scans similar to SQLite’s approach.

### 9.3 Full-text in Postgres

We emulate “multi-column FTS with weights” using stored `tsvector` columns.

Per text field `<f>` in schema:

* Column: `<f>_tsv tsvector`
* Column: `<f>_txt text` (optional, but helpful for rebuild/debug)

On put:

* store `<f>_txt`
* compute `<f>_tsv = to_tsvector('simple', coalesce(<f>_txt,''))`

Indexes:

* `GIN` on each `<f>_tsv`.

Query compilation:

* Bare text expands across all text fields: `(title_tsv @@ q) OR (content_tsv @@ q) ...`
* Fielded text targets one field’s tsv.
* Phrase search uses `phraseto_tsquery('simple', phrase)` (best-effort parity).
* Prefix search uses `to_tsquery('simple', 'alloc:*')` when term ends with `*`.

Ranking:

* Default rank score = `SUM(ts_rank_cd(<f>_tsv, q) * weight_f)` across all text fields.
* Order: `score DESC, item_id ASC`.

### 9.4 Cursor logic

Same CursorPayload kinds as SQLite; short cursors stored in `cursor_store`.

---

## 10. Redis adapter (package `adapters/redis`)

### 10.1 Baseline decision

To reach near feature parity, Redis adapter is based on **RediSearch** (Redis Query Engine). This is necessary to support:

* full-text search, field scoping, weighted scoring, boolean expressions, numeric ranges, tag filtering, and cursor-based deterministic paging via `FT.AGGREGATE ... WITHCURSOR`. ([Redis][1])

If RediSearch is not present:

* adapter reports reduced `Capabilities` (no FullText, limited discover/stats).
* CLI/library returns a clear error when unsupported query features are used.

### 10.2 Data model in Redis

We store each item as a HASH and index it with RediSearch.

Keys:

* Item hash key: `ms:{index}:item:{id}`
* Path-to-id map: `ms:{index}:path2id` (HASH path → id)
* Next id counter: `ms:{index}:nextid` (STRING, INCR)
* Cursor keys (short mode): `ms:{index}:cursor:{handle}` (STRING payload JSON, TTL)
* Registry:

  * `ms:registry` (HASH indexName → schemaJSON)

Hash fields stored per item:

* `path` (string)
* `data_json` (string; original full document JSON)
* `created_at` (int ms)
* `updated_at` (int ms)
* `__id` (int; stable insertion id)
* `__present` (string; separator-joined present field names)

Plus one hash field per schema field:

* keyword: stored as separator-joined string (custom separator)
* text: stored as raw string
* number: stored as numeric string
* date: stored as epoch ms numeric string
* bool: store `0/1` numeric string

**Separator choice for multi-valued TAG fields**

* Use ASCII Unit Separator `\x1f` as TAG `SEPARATOR`, minimizing collisions with real text.
* On put: join multi values with `\x1f`.

### 10.3 RediSearch index schema

Index name: `ms:{index}:fts`

Create:

```
FT.CREATE ms:{index}:fts ON HASH PREFIX 1 ms:{index}:item: SCHEMA
  path TAG SORTABLE
  __id NUMERIC SORTABLE
  created_at NUMERIC SORTABLE
  updated_at NUMERIC SORTABLE
  __present TAG SEPARATOR "\x1f"
  <keyword fields...> TAG SEPARATOR "\x1f"
  <number fields...> NUMERIC SORTABLE
  <date fields...>   NUMERIC SORTABLE
  <bool fields...>   TAG
  <text fields...>   TEXT WEIGHT <schema weight> SORTABLE (optional)
```

Notes:

* TAG prefix matching is supported (`tech*`). ([Redis][2])
* Full-text wildcard patterns and dialect behaviors follow RediSearch query syntax. ([Redis][3])
* For deterministic paging with stable ordering, we use `FT.AGGREGATE ... WITHCURSOR`. ([Redis][4])

### 10.4 Redis “put” mechanics (atomic)

Redis needs to preserve created_at and __id on update. We do it with a Lua script:

**Lua script (conceptual steps)**

1. Input: index name, doc JSON, extracted field-value pairs, `now_ms`.
2. Extract `path`, lookup id in `path2id`.
3. If missing:

   * allocate `id = INCR nextid`
   * set `created_at = now_ms`
   * set `__id = id`
   * write `path2id[path]=id`
4. Else:

   * read existing `created_at`, `__id`
5. Set `updated_at = now_ms`
6. HSET item hash fields:

   * `path`, `data_json`, timestamps, `__id`, `__present`, plus all schema fields.

RediSearch updates indexes automatically when HASH updates occur.

### 10.5 Query translation to RediSearch

We translate normalized AST into a RediSearch query string plus an aggregation pipeline:

#### Mapping rules

* `has:field` → `@__present:{field}`
* `field:true/false` (bool) → TAG match on that bool field.
* keyword exact: `@field:{value}`
* keyword prefix (`mem*`): `@field:{mem*}` ([Redis][2])
* keyword contains (`*alloc*`) and keyword glob:

  * Use RediSearch **tag dictionary expansion**:

    * fetch values via `FT.TAGVALS ms:{index}:fts field`
    * filter in Go by substring/glob
    * enforce `MinContainsLen`, `MaxPrefixExpansion`
    * compile to `@field:{v1 | v2 | ...}`
* text fielded: `@title:term`, phrase: `@content:"error handling"`
* bare text: `term` / `"phrase"` (no field modifier)
* number/date comparisons:

  * RediSearch numeric ranges: `@priority:[(5 +inf]` etc
  * date fields are numeric epoch ms, same syntax.
* implicit `created`/`updated` map to `created_at` / `updated_at`.

Boolean operators:

* AND is implicit by concatenation
* OR uses `|`
* NOT uses `-` prefix (dialect behavior differences exist; we always emit DIALECT >= 2 and parenthesize to be safe). ([Redis][3])

#### Ranking + pagination pipeline

We run **FT.AGGREGATE** with:

* `DIALECT 2` (or 3+ if needed for multi-values)
* `ADDSCORES` when rank=default (exposes `@__score`) ([Redis][1])
* `SORTBY` depending on rank mode (multiple keys supported): ([Redis][1])

Ranking modes:

* `default`:

  * `ADDSCORES SORTBY 4 @__score DESC @__id ASC`
* `recency`:

  * `SORTBY 4 @updated_at DESC @path ASC`
* `field:<name>`:

  * `SORTBY 6 @<field> DESC @updated_at DESC @path ASC`
* `none`:

  * `SORTBY 2 @__id ASC`

Pagination:

* Use `WITHCURSOR COUNT <limit+1> MAXIDLE <ttl_ms>` to obtain a RediSearch cursor id. ([Redis][1])

Cursor payload for Redis:

* `CursorPayload.Kind="redis"`
* store `RedisCursorID` and `RedisIndexName` (the `ms:{index}:fts` name)
* the cursor hash still validates schema+query+rank

Next page:

* call `FT.CURSOR READ <index> <cursor_id> COUNT <limit+1>`
* when exhausted, delete cursor and return no next_cursor.

Cursor modes:

* **short**: store payload JSON in `ms:{index}:cursor:<handle>` with TTL; return `c:<handle>`.
* **full**: base64 encode `CursorPosition` with Redis cursor fields included.

This preserves the “opaque cursor token” UX and avoids offset pagination.

### 10.6 Post-filter for complex path globs

For patterns like `path:/docs/*/intro`:

1. Add prefix clause into RediSearch query to bound candidate set: `@path:{/docs/*}`
2. As results stream from cursor:

   * apply Go glob match against returned `path`
   * keep collecting until `limit` matches are produced or cursor exhausted

This mirrors the SQL “prefix candidate + post-filter” strategy.

### 10.7 Discover + stats in Redis

We implement these using aggregation pipeline primitives:

* Discover values for keyword field:

  * `FT.AGGREGATE idx <scopeQuery> GROUPBY 1 @field REDUCE COUNT 0 AS cnt SORTBY 2 @cnt DESC MAX top LIMIT 0 top`
* Stats for numeric/date:

  * `FT.AGGREGATE idx <scopeQuery> REDUCE MIN 1 @field AS min REDUCE MAX 1 @field AS max REDUCE AVG 1 @field AS avg`
    Aggregation building blocks and performance notes follow Redis docs (LOAD costs, SORTBY, MAX, etc.). ([Redis][1])

---

## 11. CLI design (package `internal/cli`)

### 11.1 Global flags (all commands)

* `--backend sqlite|postgres|redis` (default `sqlite`)
* SQLite:

  * `--sqlite-dir <dir>` (default `.`)
* Postgres:

  * `--pg-dsn <dsn>`
* Redis:

  * `--redis-addr <host:port>`
  * `--redis-password <pw>`
  * `--redis-db <n>`
* Common:

  * `-i, --index <name>`
  * `--format pretty|paths|json`
  * `--explain`

### 11.2 Command surface (same verbs/nouns as Rust)

* `index create|list|schema|migrate|stats|optimize|drop`
* `put|get|peek|delete|search|discover|stats`

Implementation approach:

* CLI constructs `OpenOptions` from flags → `ministore.Open`.
* `--index` selects the logical index name:

  * SQLite: resolved to `<sqlite-dir>/<index>.db` (unless user supplies `--index` ending with `.db`, in which case treat as explicit path).
  * Postgres/Redis: index name is namespace key.

Output modes:

* `paths`: print `path` per line
* `pretty`: bullet list + optional shown fields + cursor line
* `json`: object containing `items`, `next_cursor`, and `explain_*` when requested.

---

## 12. Explain output contract

All backends return:

* `ExplainPlan`: semantic steps from `plan` (predicate compilation + boolean ops).
* `ExplainQuery`:

  * SQLite/Postgres: final SQL string
  * Redis: the exact `FT.AGGREGATE ...` command arguments string (or a structured rendering)

---

## 13. Schema evolution + migration

### 13.1 Additive apply

Allowed:

* Add new fields.
* Keyword/number/date: no rebuild needed.
* Text fields:

  * SQLite: `ALTER TABLE search ADD COLUMN <field>`
  * Postgres: add `<field>_txt`, `<field>_tsv`, and index
  * Redis: `FT.ALTER ... SCHEMA ADD ...`

All adapters must update stored `schema_json` in registry/meta and update in-memory schema in the opened `Index`.

### 13.2 Type change / multi→single

Requires rebuild:

* SQLite: create new `.db` and reindex, then swap if CLI requests.
* Postgres: create new schema namespace `ms_<index>__migr_<ts>`, reindex, then swap registry pointer (or drop old and rename schema).
* Redis: create new `ms:{newIndex}:...`, reindex by streaming old docs, then optional swap by renaming registry entry.

CLI recommendation:

* `ministore index migrate -i docs --schema new.json --to docs_v2`
* optional `--swap` to drop old and rename new.

---

## 14. Redis design space (what’s possible, what’s risky)

### 14.1 Options considered

1. **Pure Redis (sets/zsets + custom token index)**

   * Would require implementing BM25-like scoring, phrase search, field scoping, and efficient paging manually.
   * High complexity; hard to match parity.
2. **Redis + RediSearch (chosen)**

   * Built-in text search, scoring, boolean logic, numeric ranges, tag filtering, and cursor-based paging via aggregation. ([Redis][1])
   * Remaining gaps (like keyword contains on TAG) handled by dictionary expansion via `FT.TAGVALS` with guardrails.

### 14.2 Known tradeoffs

* TAG contains/glob is not a first-class “fast operator”; we use tag dictionary expansion + OR list. This preserves semantics but can be expensive on huge vocabularies; guardrails apply.
* Cursor paging relies on RediSearch cursor API (backend holds cursor state). Tokens remain opaque and self-contained for clients.

---

## 15. Testing strategy (contract-level)

### 15.1 Unit tests (backend-agnostic)

* Lexer/parser precedence, quoting, numeric/date parsing, relative date mapping.
* Normalize guardrails (positive anchor, prefix/contains min lengths).
* Plan type resolution (fielded text vs keyword, date equality coercion, bool coercion).

### 15.2 Adapter contract tests

A shared test suite runs against each backend:

* create/open schema persistence
* put/get/update timestamps
* delete by path
* search correctness for each predicate type
* boolean combinations
* rank modes ordering + cursor pagination stability
* discover values scoped and unscoped
* stats scoped and unscoped

Implementation detail:

* Postgres + Redis tests run via `testcontainers-go` in CI; SQLite uses temp files.

---

## 16. File-by-file responsibilities (quick index)

### Library

* `pkg/ministore/errors.go`
  `type Error` enum-like with wrapped causes; error codes: Schema, QueryParse, QueryRejected, Cursor, NotFound, Backend.
* `pkg/ministore/constants.go`
  guardrails + defaults.
* `pkg/ministore/index/index.go`
  high-level methods call parse/normalize/compile then delegate to store.
* `pkg/ministore/query/*`
  lexer/parser/normalize.
* `pkg/ministore/plan/*`
  schema-aware coercions (text/keyword/date/bool), explain steps.
* `pkg/ministore/cursor/*`
  hash + encode/decode.

### SQLite adapter

* `adapters/sqlite/ddl.go` base tables + fts ddl generator
* `adapters/sqlite/sql.go` SQL fragments
* `adapters/sqlite/fts.go` verify/apply schema for FTS
* `adapters/sqlite/sqlite.go` store implementation

### Postgres adapter

* `adapters/postgres/registry.go` global index registry
* `adapters/postgres/ddl.go` per-index schema creation
* `adapters/postgres/fts.go` tsv columns + gin indexes
* `adapters/postgres/postgres.go` store implementation

### Redis adapter

* `adapters/redis/schema_fts.go` FT.CREATE/ALTER generation
* `adapters/redis/query_fts.go` AST → RediSearch query + aggregate pipeline
* `adapters/redis/lua.go` put/delete scripts
* `adapters/redis/cursor_store.go` short cursor storage
* `adapters/redis/registry.go` registry in `ms:registry`

---

## 17. Concrete execution flow (end-to-end)

### `Index.Search(query, opts)`

1. `query.Parse(queryString)` → `query.Expr`
2. `query.Normalize(expr)` (guardrails)
3. `plan.Compile(schema, expr, nowMs)` → `plan.CompiledQuery`
4. If `opts.After != ""`:

   * decode cursor (short→backend store lookup; full→base64 decode)
   * validate hash matches `(schema_json, query, rank)`
5. Delegate: `store.Search(ctx, compiled, opts)`
6. Adapter returns:

   * results (already shaped by `Show`)
   * cursor token (short/full)
   * explain info if requested

---

[1]: https://redis.io/docs/latest/commands/ft.aggregate/ "https://redis.io/docs/latest/commands/ft.aggregate/"
[2]: https://redis.io/docs/latest/develop/ai/search-and-query/advanced-concepts/tags/ "https://redis.io/docs/latest/develop/ai/search-and-query/advanced-concepts/tags/"
[3]: https://redis.io/docs/latest/develop/ai/search-and-query/advanced-concepts/query_syntax/ "https://redis.io/docs/latest/develop/ai/search-and-query/advanced-concepts/query_syntax/"
[4]: https://redis.io/docs/latest/commands/ft.search/ "https://redis.io/docs/latest/commands/ft.search/"
