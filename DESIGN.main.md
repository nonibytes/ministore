# Ministore-Go Design Document (Near-Implementation Spec)

A general-purpose, schema-driven search index with boolean queries, cursor pagination, and a **pluggable SQL backend**. Default adapters: **SQLite** and **PostgreSQL**. Library-first (embeddable), CLI included, and structured so it can be used in a server easily.

This document is deliberately “close to implementation”: concrete folders/files, structs, function signatures, SQL table shapes, and mechanics.

---

## 0) Goals and non-goals

### Goals

* One “index” per SQLite file OR per Postgres schema/namespace.
* Explicit schema stored in DB meta; strict validation on insert.
* Typed storage:

  * keyword → dictionary + postings + doc_freq
  * number/date/bool → typed tables with indexes
  * text → backend-specific FTS with multi-column semantics
* Query language:

  * boolean ops `& | ! ( )`
  * fielded predicates + bare-text shorthand
  * keyword wildcards `* ?` (keyword + path only)
  * number/date comparisons and ranges
  * date relative durations (`<7d`, `>30d`) with defined semantics
  * existence `has:field`
* Query planning: compile to **CTE set algebra** (`INTERSECT/UNION/EXCEPT`).
* Ranking:

  * default: weighted relevance
  * recency: updated_at desc
  * field: numeric/date desc (plus tie-breakers)
  * none: stable insertion order
* Cursor pagination:

  * full cursor: stateless base64url JSON
  * short cursor: handle stored in DB with TTL

### Non-goals (v1)

* Cross-index queries / workspace aggregation
* Trigram contains indexes
* Rich aggregations beyond discover/stats
* Advanced linguistic features (synonyms, stemming controls beyond backend defaults)

---

## 1) Repository and package layout

```
ministore-go/
  go.mod

  cmd/
    ministore/
      main.go

  internal/
    cli/
      root.go
      resolve.go
      output/
        format.go
        pretty.go
        json.go
      commands/
        index.go
        put.go
        get.go
        peek.go
        delete.go
        search.go
        discover.go
        stats.go

  ministore/                      // public API: import "ministore"
    constants.go
    errors.go
    types.go
    schema.go
    item.go
    batch.go
    cursor.go
    index.go

    query/
      ast.go
      lexer.go
      parser.go
      normalize.go

    planner/
      cte.go
      compile.go
      sqlbuild.go
      after.go
      explain.go

    ops/
      put.go
      delete.go
      search.go
      discover.go
      stats.go
      migrate.go
      meta.go
      util.go

    storage/
      adapter.go
      sqlbuilder/
        builder.go
      sqlite/
        adapter.go
        ddl.go
        sql.go
        fts.go
      postgres/
        adapter.go
        ddl.go
        sql.go
        fts.go
```

* `ministore/` contains the embeddable library.
* `internal/cli` is the CLI app only.
* `storage/*` contains adapters.
* `query/*` and `planner/*` are backend-neutral.

---

## 2) Core concepts mapped to DB

### 2.1 Logical tables (common)

All backends must provide these logical tables (names may be schema-qualified in Postgres):

* `meta(key TEXT PRIMARY KEY, value TEXT)`
* `items(id PK, path UNIQUE, data_json, created_at, updated_at)`
* `field_present(item_id, field, PRIMARY KEY(item_id, field))`
* `kw_dict(id PK, field, value, doc_freq, UNIQUE(field,value))`
* `kw_postings(field, value_id, item_id, PRIMARY KEY(value_id,item_id))`
* `field_number(item_id, field, value, PRIMARY KEY(item_id,field,value))`
* `field_date(item_id, field, value, PRIMARY KEY(item_id,field,value))`
* `field_bool(item_id, field, value, PRIMARY KEY(item_id,field))`
* `cursor_store(handle PK, payload, created_at, expires_at)`
* full-text structure: `search` (backend-specific)

### 2.2 Type routing rules

On insert/update:

* **keyword** → `kw_dict` + `kw_postings` + `field_present`
* **text** → backend FTS structure + `field_present`
* **number** → `field_number` + `field_present`
* **date** → `field_date` + `field_present`
* **bool** → `field_bool` + `field_present`

---

## 3) Public API (library)

### 3.1 ministore/types.go

```go
type CursorMode string
const (
  CursorShort CursorMode = "short"
  CursorFull  CursorMode = "full"
)

type RankModeKind string
const (
  RankDefault RankModeKind = "default"
  RankRecency RankModeKind = "recency"
  RankField   RankModeKind = "field"
  RankNone    RankModeKind = "none"
)

type RankMode struct {
  Kind  RankModeKind
  Field string // only if Kind==RankField
}

type OutputFieldSelectorKind string
const (
  ShowNone   OutputFieldSelectorKind = "none"
  ShowAll    OutputFieldSelectorKind = "all"
  ShowFields OutputFieldSelectorKind = "fields"
)

type OutputFieldSelector struct {
  Kind   OutputFieldSelectorKind
  Fields []string
}

type IndexOptions struct {
  CursorTTL          time.Duration // default 1h
  Now                func() time.Time // default time.Now
  MinContainsLen     int // default 3
  MinPrefixLen       int // default 2
  MaxPrefixExpansion int // default 20000
}

type SearchOptions struct {
  Rank       RankMode
  Limit      int
  After      string // cursor token or ""
  CursorMode CursorMode
  Show       OutputFieldSelector
  Explain    bool
}
```

### 3.2 ministore/index.go

```go
type Index struct {
  adapter storage.Adapter
  db      *sql.DB
  schema  Schema
  opts    IndexOptions
}

func Create(ctx context.Context, adapter storage.Adapter, schema Schema, opts IndexOptions) (*Index, error)
func Open(ctx context.Context, adapter storage.Adapter, opts IndexOptions) (*Index, error)

func (ix *Index) Close() error
func (ix *Index) Schema() Schema

func (ix *Index) PutJSON(ctx context.Context, docJSON []byte) error
func (ix *Index) PutFields(ctx context.Context, path string, fieldsJSON []byte) error

func (ix *Index) Get(ctx context.Context, path string) (ItemView, error)
func (ix *Index) Peek(ctx context.Context, path string) ([]byte, error)

func (ix *Index) Delete(ctx context.Context, path string) (bool, error)
func (ix *Index) DeleteWhere(ctx context.Context, query string) (int, error)

func (ix *Index) Search(ctx context.Context, query string, opts SearchOptions) (SearchResultPage, error)

func (ix *Index) DiscoverValues(ctx context.Context, field string, where string, top int) ([]ValueCount, error)
func (ix *Index) DiscoverFields(ctx context.Context) ([]FieldOverview, error)
func (ix *Index) Stats(ctx context.Context, field string, where string) (StatsResult, error)

func (ix *Index) Optimize(ctx context.Context) error
func (ix *Index) ApplySchema(ctx context.Context, newSchema Schema) error
func (ix *Index) MigrateRebuild(ctx context.Context, dst storage.Adapter, newSchema Schema) error

func (ix *Index) Batch(ctx context.Context, b Batch) (int, error)
```

### 3.3 ministore/item.go

```go
type ItemMeta struct {
  CreatedAtMS int64
  UpdatedAtMS int64
}
type ItemView struct {
  Path   string
  DocJSON []byte
  Meta   ItemMeta
}
```

### 3.4 ministore/index.go outputs

```go
type SearchResultPage struct {
  Items       [][]byte // output-shaped JSON per item
  NextCursor  string
  HasMore     bool
  ExplainSQL  string
  ExplainSteps []string
}

type ValueCount struct { Value string; Count uint64 }

type FieldOverview struct {
  Field    string
  Type     FieldType
  Multi    bool
  DocCount uint64
  Unique   *uint64
  Weight   *float64
  Examples []string
}

type StatsResult struct {
  Field string
  Count uint64
  Min   *float64
  Max   *float64
  Avg   *float64
  Median *float64
}
```

---

## 4) Error model (ministore/errors.go)

```go
type ErrorKind string
const (
  ErrSQL           ErrorKind = "sql"
  ErrSchema        ErrorKind = "schema"
  ErrQueryParse    ErrorKind = "query_parse"
  ErrQueryRejected ErrorKind = "query_rejected"
  ErrUnknownField  ErrorKind = "unknown_field"
  ErrTypeMismatch  ErrorKind = "type_mismatch"
  ErrCursor        ErrorKind = "cursor"
  ErrNotFound      ErrorKind = "not_found"
  ErrFeature       ErrorKind = "feature_missing"
)

type Error struct {
  Kind    ErrorKind
  Message string
  Field   string
  Cause   error
}
func (e *Error) Error() string
func Wrap(kind ErrorKind, msg string, cause error) *Error
func TypeMismatch(field, msg string) *Error
```

All public methods return `error`, but errors are normally `*ministore.Error`.

---

## 5) Schema (ministore/schema.go)

### 5.1 Types

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

func (s Schema) Validate() error
func (s Schema) ToJSON() ([]byte, error)
func SchemaFromJSON(b []byte) (Schema, error)

type TextField struct { Name string; Weight float64 }
func (s Schema) TextFieldsInOrder() []TextField
func (s Schema) Get(name string) (FieldSpec, bool)
func (s Schema) HasField(name string) bool
```

### 5.2 Validation rules

* schema must have ≥ 1 field
* field name regex `^[A-Za-z_][A-Za-z0-9_]*$`
* reserved names: `path`, `created`, `updated`
* weight only for text; weight > 0

---

## 6) Constants (ministore/constants.go)

```go
const DefaultMinContainsLen = 3
const DefaultMinPrefixLen   = 2
const DefaultMaxPrefixExpansion = 20000
const DefaultCursorTTL = time.Hour
```

---

## 7) Cursor mechanics (ministore/cursor.go)

### 7.1 Payload

```go
type CursorPayloadKind string
const (
  CurFts     CursorPayloadKind = "fts"
  CurRecency CursorPayloadKind = "recency"
  CurField   CursorPayloadKind = "field"
  CurNone    CursorPayloadKind = "none"
)

type CursorPayload struct {
  Kind CursorPayloadKind `json:"kind"`

  // FTS
  Score  float64 `json:"score,omitempty"`
  ItemID int64   `json:"item_id,omitempty"`

  // Recency/Field
  UpdatedAtMS int64  `json:"updated_at_ms,omitempty"`
  Path        string `json:"path,omitempty"`

  // Field
  Field     string  `json:"field,omitempty"`
  RankValue float64 `json:"rank_value,omitempty"`
}

type CursorPosition struct {
  Payload CursorPayload `json:"payload"`
  Hash    string        `json:"hash"`
}
```

### 7.2 Cursor functions

```go
func HashQuery(schemaJSON []byte, query string, rank RankMode) string // sha256 hex

func EncodeFull(pos CursorPosition) (string, error) // base64url no pad
func DecodeFull(token string) (CursorPosition, error)

func IsShortCursorToken(token string) bool // "c:"
func ShortHandle(token string) (string, bool) // strip "c:"
func MakeShortHandle() (string, error) // 12 random bytes -> hex
```

### 7.3 Short cursor store semantics

* token: `c:<handle>`
* store: payload JSON + expires_at (now + ttl)
* cleanup: `DELETE ... WHERE expires_at < now` at start of Search()

---

## 8) Storage adapter architecture

### 8.1 Placeholder handling (storage/sqlbuilder/builder.go)

```go
type PlaceholderStyle int
const (
  PlaceholderQuestion PlaceholderStyle = iota // "?"
  PlaceholderDollar                           // "$1"
)

type Builder struct {
  Style PlaceholderStyle
  args  []any
}
func New(style PlaceholderStyle) *Builder
func (b *Builder) Arg(v any) string // returns placeholder string and appends v
func (b *Builder) Args() []any
func (b *Builder) Len() int
```

### 8.2 Adapter interface (storage/adapter.go)

The adapter surface is intentionally small: DDL/meta/FTS/upsert quirks.

```go
type Backend string
const (
  BackendSQLite Backend = "sqlite"
  BackendPostgres Backend = "postgres"
)

type Adapter interface {
  Backend() Backend
  PlaceholderStyle() PlaceholderStyle
  IndexID() string

  Connect(ctx context.Context) (*sql.DB, error)
  Close() error

  // index lifecycle: create/open/verify
  CreateIndex(ctx context.Context, db *sql.DB, schemaJSON []byte) error
  OpenIndex(ctx context.Context, db *sql.DB) (schemaJSON []byte, err error)

  VerifyFTS(ctx context.Context, db *sql.DB, schema ministore.Schema) error
  ApplySchemaAdditive(ctx context.Context, db *sql.DB, old, new ministore.Schema) error

  Optimize(ctx context.Context, db *sql.DB) error

  // migration helper: stream out of src and rebuild into dst
  RebuildInto(ctx context.Context, srcDB *sql.DB, dst Adapter, newSchemaJSON []byte, nowMS func() int64) error

  SQL() SQLTemplates
  FTS() FTSDriver
}

type SQLTemplates struct {
  // meta
  GetMeta string
  SetMeta string

  // items
  FindItemIDByPath string
  GetItemByPath string

  // cursor
  CleanupExpiredCursors string
  GetCursor string
  PutCursor string

  // delete by item
  DeleteSearchRow string
  DeletePresentByItem string
  DeletePostingsByItem string
  DeleteNumberByItem string
  DeleteDateByItem string
  DeleteBoolByItem string
  DeleteItemsByID string

  // keyword doc_freq maintenance
  GetValueIDsByItem string
  IncDocFreq string
  DecDocFreq string

  // keyword inserts/lookup
  InsertOrIgnoreKwDict string
  GetKwDictID string
  InsertOrIgnoreKwPosting string

  // typed inserts
  InsertFieldPresent string
  InsertFieldNumber string
  InsertFieldDate string
  InsertFieldBool string

  // item upsert is backend-specific (two-step in sqlite, RETURNING in pg)
  UpsertItem func(b *sqlbuilder.Builder, path string, dataJSON []byte, nowMS int64) (sql string)
  UpsertItemWithTimestamps func(b *sqlbuilder.Builder, path string, dataJSON []byte, createdMS, updatedMS int64) (sql string)
}

type FTSDriver interface {
  HasFTS(schema ministore.Schema) bool
  CreateFTS(ctx context.Context, db *sql.DB, schema ministore.Schema) error
  VerifyFTS(ctx context.Context, db *sql.DB, schema ministore.Schema) error
  AddTextColumns(ctx context.Context, db *sql.DB, old, new ministore.Schema) error

  DeleteFTSRow(ctx context.Context, tx *sql.Tx, itemID int64) error
  UpsertFTSRow(ctx context.Context, tx *sql.Tx, itemID int64, schema ministore.Schema, text map[string]*string) error

  // compile a single text predicate into a CTE that yields item_id
  CompileTextPredicate(b *sqlbuilder.Builder, schema ministore.Schema, field *string, q string) (cteSQL string)

  // provide ranking join/expression for RankDefault
  // given list of text preds (field-scoped or bare)
  BuildScoreSupport(b *sqlbuilder.Builder, schema ministore.Schema, textPreds []planner.TextPredicate) (extraCTEs []planner.CTE, joinSQL string, scoreExpr string, err error)
}
```

---

## 9) Backend DDL and SQL templates

## 9.1 SQLite (storage/sqlite)

### 9.1.1 DDL (ddl.go)

* Use integer PK and sqlite semantics.
* Base DDL mirrors the Rust implementation.
* `items.data_json` stored as TEXT, but library uses `[]byte` and passes string/[]byte as driver supports.

Key indexes:

* `idx_items_path`, `idx_items_updated`, `idx_items_created`
* `idx_present_field(field, item_id)`
* `idx_kw_dict_lookup(field,value)`
* `idx_kw_postings_item(item_id)`
* `idx_kw_postings_field(field,value_id)`
* `idx_num_lookup(field,value)`
* `idx_date_lookup(field,value)`
* `idx_bool_lookup(field,value)`
* `idx_cursor_expires(expires_at)`

FTS DDL generated from schema text fields:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS search USING fts5(col1, col2, ..., tokenize='unicode61');
```

### 9.1.2 SQLite FTS implementation (fts.go)

* Verify FTS5 availability: `pragma_compile_options` contains `ENABLE_FTS5` if possible; if not possible, fallback by attempting to create fts table and error.
* Upsert row:

  1. `DELETE FROM search WHERE rowid=?`
  2. `INSERT INTO search(rowid, col1, col2, ...) VALUES(?, ?, ...)`
* MATCH string rules:

  * fielded query uses `field:term`
  * bare text expands to `(col1:term OR col2:term ...)`
  * quote terms containing whitespace or reserved chars: `"error handling"` with escaping `""` for internal `"`.

Default ranking:

* Use FTS5 `bm25(search, w1, w2, ...)` where weights come from schema.
* Convert to “higher is better”: `score = -bm25(search, ...)`.

### 9.1.3 SQLite SQLTemplates (sql.go)

* `InsertOrIgnoreKwDict`: `INSERT OR IGNORE INTO kw_dict(field,value,doc_freq) VALUES(?,?,0)`
* `InsertOrIgnoreKwPosting`: `INSERT OR IGNORE INTO kw_postings(field,value_id,item_id) VALUES(?,?,?)`
* Meta upsert: `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`
* Cursor store insert: simple insert; handle collisions by retry.

Item upsert strategy (no single statement returning created_at reliably):

* `SELECT id, created_at FROM items WHERE path=?`
* if exists: `UPDATE items SET data_json=?, updated_at=? WHERE id=?` return existing id, created_at
* else: `INSERT INTO items(path,data_json,created_at,updated_at) VALUES(?,?,?,?)` and `last_insert_rowid()`

(Implemented via `SQLTemplates.UpsertItem` function that emits either a known SQL block or uses ops-level logic; v1 uses ops-level two-step for sqlite.)

---

## 9.2 Postgres (storage/postgres)

### 9.2.1 Namespace model

One “index” is a **schema**: `ministore_<indexname>`.

* All tables are created inside this schema.
* Adapter stores schema name and qualifies every SQL statement with it.

### 9.2.2 DDL differences

* `items.id` is `BIGSERIAL` (or `GENERATED BY DEFAULT AS IDENTITY`)
* `data_json` is `JSONB`
* `created_at/updated_at` stored as `BIGINT` epoch-ms
* `kw_dict.value` is `TEXT`, `doc_freq` is `BIGINT`

Indexes are same logical shape; use B-tree and GIN for FTS.

### 9.2.3 Postgres FTS structure

Table `search`:

* `item_id BIGINT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE`
* for each text field `title`: `title_tsv tsvector NOT NULL`
* Create a GIN index per column: `CREATE INDEX ... USING GIN(title_tsv)`

No triggers required; app computes vectors on put.

### 9.2.4 Postgres FTS queries and ranking

* term/phrase compilation:

  * phrase `"error handling"` → `phraseto_tsquery(config, 'error handling')`
  * plain term `memory` → `plainto_tsquery(config, 'memory')`
  * prefix `alloc*` → `to_tsquery(config, 'alloc:*')`
* field-scoped:

  * `title_tsv @@ <tsquery>`
* bare text expands OR across all text fields.

Default ranking:

* `ts_rank_cd(field_tsv, tsquery)` per field
* weighted sum by schema weights:

  * `score = w_title*ts_rank_cd(title_tsv, q) + w_body*...`
* If multiple text predicates exist, sum their scores:

  * build per-predicate CTE returning (item_id, score) and then aggregate `SUM(score)`.

---

## 10) Query subsystem

## 10.1 AST (query/ast.go)

Direct analog of Rust.

* Expr:

  * And, Or, Not, Pred
* Predicate:

  * Has
  * PathGlob
  * Keyword(field, pattern, kind)
  * Text(field optional, fts string)
  * NumberCmp, NumberRange
  * DateCmpAbs, DateRangeAbs, DateCmpRel
  * Bool

## 10.2 Lexer (query/lexer.go)

Tokens:

* Ident, String, Number
* Colon, And, Or, Not, LParen, RParen
* Gt, Gte, Lt, Lte
* DotDot

Rules:

* word tokens stop at operator boundaries and before `..`.
* quoted strings support escapes `\" \\ \n \t \r`.

## 10.3 Parser (query/parser.go)

Precedence:

* `Not` highest, then `And`, then `Or`.

Shorthand:

* `!archived` or `NOT archived` becomes `Bool(field="archived", value=false)` only if the ident is not followed by `:` or comparison operator.

Field predicates:

* `has:<field>` produces Has predicate.
* `path:<pattern>` produces PathGlob.
* `field:value` initially produces `Keyword` predicate (planner will reinterpret based on schema type: text/bool/date coercions).
* `field:1..10` produces NumberRange
* comparisons: `field>5`, `due<7d`, `created>2024-01-01` produce NumberCmp or DateCmpAbs/Rel.

## 10.4 Normalize (query/normalize.go)

Enforces:

* positive anchor exists:

  * any Text predicate
  * exact keyword match
  * numeric/date predicates
  * path pattern with literal prefix before wildcard
* guardrails:

  * prefix too short
  * contains too short
  * glob must have literal prefix before first wildcard and meet min length

---

## 11) Planner (compile to CTE set algebra)

### 11.1 planner/compile.go

Inputs:

* schema
* normalized AST
* now_ms (for relative dates)

Output:

```go
type CompileOutput struct {
  CTEs []CTE
  ResultCTE string
  Args []any
  ExplainSteps []string
  TextPreds []TextPredicate // extracted for RankDefault scoring
}
```

Compiler algorithm:

* Each predicate compiles into a CTE `cte_n` producing `item_id`:

  * `SELECT item_id FROM ...`
* Boolean combination:

  * AND → new CTE with `INTERSECT`
  * OR → new CTE with `UNION`
  * NOT → new CTE with `SELECT id AS item_id FROM items EXCEPT SELECT item_id FROM inner`
* Store explain step strings like Rust.

### 11.2 Predicate compilation (planner/sqlbuild.go uses adapter FTS)

Backend-neutral SQL except where noted:

* Has:

  * `SELECT item_id FROM field_present WHERE field = <arg>`

* PathGlob:

  * prefix-only pattern `/docs/*`:

    * SQLite: `items.path LIKE '/docs/%'`
    * Postgres: same
  * internal wildcard pattern:

    * Candidate prefix: literal part up to first wildcard → `LIKE '<prefix>%'`
    * Post-filter:

      * SQLite: `GLOB '<pattern>'`
      * Postgres: translate glob to LIKE: `*`→`%`, `?`→`_` with escaping of `% _ \`.

* Keyword:

  * dict + postings:

    * exact: `d.value = ?`
    * prefix: `d.value LIKE 'mem%'`
    * contains: `d.value LIKE '%alloc%'`
    * glob:

      * SQLite: `d.value GLOB ?`
      * Postgres: translate to LIKE (same as path)
  * SQL form:

    ```sql
    SELECT p.item_id
    FROM kw_dict d
    JOIN kw_postings p ON p.value_id = d.id
    WHERE d.field = <fieldArg> AND <valueClause>
    ```

* Keyword predicate reinterpretation based on schema type:

  * if schema[field] is Text → becomes Text predicate (field-scoped)
  * if schema[field] is Bool and value is true/false → Bool predicate
  * if schema[field] is Date and kind is exact → DateCmpAbs(Eq)
  * if field is implicit `created`/`updated`:

    * exact date becomes `items.created_at = epoch_ms` or `items.updated_at = epoch_ms`
    * wildcards rejected

* Text:

  * delegated to adapter FTS:

    * CTE SQL = `FTSDriver.CompileTextPredicate(...)`
  * Add to `TextPreds` for RankDefault scoring.

* NumberCmp / Range:

  * `SELECT item_id FROM field_number WHERE field=? AND value op ?`
  * range uses inclusive bounds.

* DateCmpAbs / Range:

  * implicit created/updated: operate on `items.created_at` / `items.updated_at`
  * otherwise: `field_date`
  * range inclusive.

* DateCmpRel:

  * Convert amount/unit to duration_ms with approximations for M/Y:

    * M=30d, Y=365d (matches Rust)
  * Semantics:

    * if implicit field created/updated: treat as “age”

      * `<Nd` means within last Nd → timestamp >= now - Nd
      * `>Nd` means older → timestamp <= now - Nd
      * map operator accordingly (Lt/Lte → Gte, Gt/Gte → Lte)
    * schema date fields: treat as offset from now:

      * `due:<7d` means due < now + 7d (no operator inversion)

* Bool:

  * `SELECT item_id FROM field_bool WHERE field=? AND value=?`

---

## 12) Search SQL building (ranking + pagination)

### 12.1 planner/sqlbuild.go: build final SQL

Inputs:

* compiled CTEs (ResultCTE)
* rank mode
* limit+1
* optional after-filter fragment + args (built by planner/after.go)

Outputs:

* SQL string
* args list

**General structure (required for score-in-WHERE paging):**

```sql
WITH cte_0 AS (...), cte_1 AS (...), ...
     [extra score CTEs...]
SELECT item_id, path, data_json, created_at, updated_at, score
FROM (
  SELECT i.id AS item_id, i.path, i.data_json, i.created_at, i.updated_at,
         <score_expr> AS score
  FROM items i
  JOIN <result_cte> r ON r.item_id = i.id
  <optional joins: search/score tables/rank_field>
) q
WHERE 1=1 AND (<after_filter>)
ORDER BY <order_clause>
LIMIT <limit_plus_one>
```

### 12.2 Rank modes and tie-breakers

* RankNone:

  * ORDER BY `item_id ASC`
  * cursor payload: `{kind:none, item_id:lastID}`

* RankRecency:

  * ORDER BY `updated_at DESC, path ASC`
  * cursor payload: `{kind:recency, updated_at_ms:lastUpdated, path:lastPath}`

* RankField(field):

  * add CTE `rank_field`:

    * number: `SELECT item_id, MAX(value) AS rank_value FROM field_number WHERE field=? GROUP BY item_id`
    * date: same on `field_date`
  * join rank_field
  * score_expr = `CAST(rank_field.rank_value AS REAL)` (or numeric)
  * ORDER BY `score DESC, updated_at DESC, path ASC`
  * cursor payload: `{kind:field, field, rank_value:score, updated_at_ms, path}`

* RankDefault:

  * if compiled query includes any text predicates and schema has text fields:

    * adapter supplies:

      * extra score CTEs
      * join SQL
      * score_expr
    * ORDER BY `score DESC, item_id ASC`
    * cursor payload: `{kind:fts, score, item_id}`
  * else fallback:

    * ORDER BY `updated_at DESC, path ASC`
    * cursor payload: recency kind

### 12.3 planner/after.go: after-filter fragments

Fragments must exactly match ORDER BY:

* None:

  * `item_id > ?`

* Recency:

  * `(updated_at < ? OR (updated_at = ? AND path > ?))`

* Field:

  * `(score < ? OR (score = ? AND (updated_at < ? OR (updated_at = ? AND path > ?))))`

* Default FTS:

  * `(score < ? OR (score = ? AND item_id > ?))`

`after.go` also validates cursor payload kind matches rank mode, otherwise cursor error.

---

## 13) Put pipeline (ops/put.go)

### 13.1 Preparation: validate/coerce without rewriting stored JSON

`preparePut(schema, docJSON) -> PutPrepared`

```go
type PutPrepared struct {
  Path string
  DataJSON []byte

  TextCols map[string]*string   // nil means absent
  KeywordFields map[string][]string
  NumberFields map[string][]float64
  DateFieldsMS map[string][]int64
  BoolFields map[string]bool

  PresentFields []string
}
```

Rules mirror Rust:

* doc must be JSON object, must contain `"path"` string non-empty
* text fields must be strings
* keyword values:

  * string / number / bool coerced to string
  * array allowed if multi or length<=1
* number values:

  * number or string parseable to float
  * array allowed with multi
* date values:

  * string parseable as `YYYY-MM-DD` or RFC3339
  * store epoch-ms for indexing; doc stored untouched
* bool values:

  * bool or `"true"/"false"` string

Presence:

* include field in `PresentFields` if it exists and has any value after coercion.

### 13.2 Execute put (transaction)

`executePut(tx, schema, prep, nowMS)`

Steps:

1. upsert items row:

   * returns `(itemID, createdAtMS)`
   * updates updated_at
2. load old keyword value_ids:

   * `SELECT value_id FROM kw_postings WHERE item_id=?`
3. delete old index rows:

   * postings, number, date, bool, present, fts row (ignore if no fts)
4. insert field_present rows
5. insert keywords:

   * insert dict (ignore conflict)
   * get dict id
   * insert posting (ignore conflict)
   * doc_freq:

     * increment only if value_id was not previously associated with this item AND not already incremented in this put (dedupe)
6. decrement doc_freq for removed value_ids
7. insert numbers/dates/bools
8. upsert fts row:

   * build per-text-field string values
   * adapter handles insert/update mechanics
9. commit

Concurrency expectations:

* SQLite: single-writer; transaction serializes changes.
* Postgres: use `READ COMMITTED` by default; doc_freq updates are safe because they are increments/decrements on single rows; correctness relies on operations being in one transaction.

---

## 14) Delete pipeline (ops/delete.go)

### 14.1 deleteByItemID(tx, itemID)

1. load `value_id`s from postings
2. decrement doc_freq per value_id (clamped at 0 for safety)
3. delete:

   * kw_postings by item
   * field_number/date/bool by item
   * field_present by item
   * fts row
   * items row

### 14.2 Delete(path)

* find item_id by path
* if found: tx deleteByItemID, return true
* else return false

### 14.3 DeleteWhere(query)

* compile query to CTE result
* select item_ids from result
* tx delete each id
* return count

---

## 15) Search pipeline (ops/search.go)

1. cleanup expired cursors (best-effort)
2. compute expected cursor hash:

   * schema_json + query + rank mode json
3. resolve `After` cursor:

   * short cursor: load payload_json, verify expires, parse CursorPosition
   * full cursor: decode base64url
4. validate cursor hash matches expected
5. parse query → normalize
6. compile CTEs (collect text preds)
7. build after-filter fragment + args
8. build final SQL via planner/sqlbuild + adapter FTS score support
9. execute query with limit+1
10. has_more = rows > limit; keep first limit
11. compute next cursor from last row (depends on rank mode)
12. shape output items:

* `ShowNone`: JSON object `{ "path": "<path>" }`
* `ShowAll`: stored doc JSON (ensure path included; if doc lacks path, inject path in output only)
* `ShowFields`: output object with `path` + requested fields extracted from doc JSON

13. if cursor_mode short: store cursor position in cursor_store with TTL; return `c:<handle>`
14. return SearchResultPage (include explain SQL/steps if requested)

---

## 16) Discover and stats (ops/discover.go, ops/stats.go)

### 16.1 DiscoverValues(field, where, top)

* validate field exists and is keyword
* if where empty:

  * select from kw_dict ordered by doc_freq desc, value asc
* if where present:

  * compile where to result CTE (no limit)
  * join postings+dict restricted to that field, group by dict value:

    * count distinct item_id
  * order and limit

### 16.2 DiscoverFields()

For each schema field:

* doc_count = count distinct in field_present
* unique keyword count (kw_dict count)
* examples:

  * keyword: first N by doc_freq
  * number/date: min/max sample
  * bool: counts true/false sample
  * text: none or “(text)”

### 16.3 Stats(field, where)

Supports:

* implicit `created` and `updated` (items columns)
* schema number/date fields (field_number/field_date)

For SQLite median:

* two queries:

  * `SELECT COUNT(*)` for scoped values
  * `SELECT value FROM (...) ORDER BY value LIMIT 1 OFFSET k` (and second for even)
    For Postgres median:
* `PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY value)`.

Return floats (epoch-ms for dates).

---

## 17) Schema evolution (ApplySchema) and migration (MigrateRebuild)

### 17.1 ApplySchema(newSchema)

Requirements:

* all existing fields must still exist with same type and multi
* text field order must be stable for SQLite FTS column expectations:

  * ordering is deterministic by field name; adding a new field adds a new column (end of sorted order may shift if name sorts earlier). **To avoid reorder breakage**, v1 rule:

    * enforce that `TextFieldsInOrder()` of old is a prefix of new in the same order.
    * This implies: only add text fields with names that sort after existing text fields. (Same constraint as “no reorder” in Rust.)
* adapter adds new FTS columns for added text fields
* update meta schema_json
* update in-memory schema

### 17.2 MigrateRebuild(dstAdapter, newSchema)

* create destination index (new adapter) with new schema
* stream all items from src:

  * `SELECT data_json, created_at, updated_at FROM items ORDER BY id`
* for each item:

  * execute put into destination preserving timestamps
* supports cross-backend migration (sqlite→pg, pg→sqlite, etc.)

---

## 18) Maintenance (Optimize)

* SQLite:

  * if FTS exists: `INSERT INTO search(search) VALUES('optimize')`
  * `VACUUM`
* Postgres:

  * `VACUUM (ANALYZE)` on key tables
  * optionally `REINDEX` on gin indexes (adapter may no-op by default)

---

## 19) CLI (cmd/ministore + internal/cli)

### 19.1 Global flags

* `--backend sqlite|postgres` (default sqlite)
* `-i, --index <name>`:

  * sqlite: resolved to `./<name>.db` unless path-like or ends in `.db`
  * postgres: used as suffix in schema name `ministore_<index>`
* postgres flags:

  * `--dsn <dsn>` (or env `MINISTORE_DSN`)
  * `--schema-prefix ministore` (default “ministore”)

### 19.2 Commands (internal/cli/commands)

* `index create`:

  * `--schema <file>` OR repeated `--field name:type[:multi]`
* `index list`:

  * sqlite: scan current dir for `.db` and attempt open (magic check)
  * pg: list schemas matching prefix
* `index schema`:

  * show schema json
  * `--apply <file>` apply additive schema
* `index migrate`:

  * required flags:

    * `--to-backend`, `--to-index`
    * if pg target: `--to-dsn`
    * `--schema <file>`
* `index optimize`
* `index drop`

Data:

* `put`:

  * single: `--path` + repeated `--set k=v`
  * jsonl: `--json` stdin OR `--import file.jsonl`
* `get --path`
* `peek --path`
* `delete --path` OR `-w/--where`
* `search -w <query>` plus:

  * `--limit`, `--after`, `--cursor short|full`, `--rank default|recency|none|field:<name>`, `--show all|f1,f2`, `--format pretty|paths|json`, `--explain`
* `discover fields`
* `discover values --field <name> [-w <query>] [--top N]`
* `stats --field <name> [-w <query>]`

### 19.3 Output formats

* `paths`: print path per line
* `pretty`: similar to Rust pretty
* `json`: emit machine-readable object:

  * `items`, `next_cursor`, optional `explain_sql`, `explain_steps`

---

## 20) Per-file “what must exist” checklist

### ministore/

* `index.go`: Create/Open wiring, public methods call `ops/*`
* `schema.go`: Schema parsing/validation + deterministic text ordering
* `cursor.go`: full/short cursor utilities + hashing
* `batch.go`: in-memory batch struct with `PutJSON`, `Delete(path)`, and execute via `Index.Batch`

### ministore/query/

* `lexer.go`: tokenize
* `parser.go`: AST build
* `normalize.go`: positive anchor + guardrails

### ministore/planner/

* `compile.go`: AST → CTE set algebra
* `sqlbuild.go`: final query assembly
* `after.go`: cursor predicate fragments

### ministore/ops/

* `put.go`: preparePut + executePut + timestamp-preserving variant for migration
* `delete.go`: deleteByItemID + delete by path/query
* `search.go`: cursor resolution + compile + execute + shape output + store short cursor
* `discover.go`: discover values + field overview
* `stats.go`: stats with median per backend capability
* `meta.go`: read/write meta keys
* `migrate.go`: rebuild logic using adapter hooks

### ministore/storage/

* `adapter.go`: interfaces + SQLTemplates + FTSDriver
* `sqlbuilder/builder.go`: placeholder manager
* `sqlite/*`: DDL + SQL strings + FTS + adapter
* `postgres/*`: DDL + SQL strings + FTS + adapter

### internal/cli/

* `root.go`: command tree + global flags
* `resolve.go`: sqlite path resolution; pg schema naming
* `commands/*.go`: each command maps to library calls
* `output/*`: formatting

---

## 21) Testing strategy (design-level)

### Unit tests (backend-neutral)

* schema validation + json roundtrip
* lexer/parser correctness (operators, quoting, numbers, dates)
* normalize guardrails and anchor rejection
* cursor encode/decode + hash binding
* planner compilation: CTE set algebra generation + args count

### Integration tests per backend

* CRUD (put/get/update/delete)
* keyword wildcards: exact/prefix/contains/glob
* date relative semantics (created/updated age vs schema date offset)
* pagination:

  * RankDefault (FTS) paging
  * RankField paging
  * RankDefault fallback recency (no text fields)
* discover and stats scoped by query

---

## 22) Key design decisions that make it extensible

* Planner produces “set of item_id” CTEs for every predicate, so new operators/predicates just need a new CTE emitter.
* FTS is isolated behind `FTSDriver` so adding a backend or swapping FTS strategy does not affect query parsing/planning structure.
* SQL placeholder differences are eliminated by `sqlbuilder.Builder`.
* All ops run through adapter-provided SQL templates for the few backend-specific bits (upsert, insert-ignore, glob matching, fts).

---

If you want the design document even closer to implementation, the next refinement would be a “statement catalog” that enumerates **every single SQL statement** (exact strings) for both adapters, including schema-qualified versions for Postgres, and a per-function “inputs/outputs/side effects” table.
