package storage

import (
	"context"
	"database/sql"

	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

type Backend string

const (
	BackendSQLite   Backend = "sqlite"
	BackendPostgres Backend = "postgres"
)

// Adapter abstracts database-specific operations
type Adapter interface {
	Backend() Backend
	PlaceholderStyle() sqlbuilder.PlaceholderStyle
	IndexID() string

	Connect(ctx context.Context) (*sql.DB, error)
	Close() error

	CreateIndex(ctx context.Context, db *sql.DB, schemaJSON []byte) error
	OpenIndex(ctx context.Context, db *sql.DB) (schemaJSON []byte, err error)
	VerifyFTS(ctx context.Context, db *sql.DB, schema Schema) error
	ApplySchemaAdditive(ctx context.Context, db *sql.DB, old, new Schema) error
	Optimize(ctx context.Context, db *sql.DB) error

	SQL() SQL
	FTS() FTS
}

// Schema is a minimal interface to avoid circular dependency
type Schema interface {
	ToJSON() ([]byte, error)
	TextFieldsInOrder() []TextField
	Get(name string) (FieldSpec, bool)
	HasField(name string) bool
}

type FieldType string

type FieldSpec struct {
	Type   FieldType
	Multi  bool
	Weight *float64
}

type TextField struct {
	Name   string
	Weight float64
}

// SQL holds prepared SQL templates for common operations
type SQL struct {
	GetMeta string
	SetMeta string

	FindItemIDByPath string
	GetItemByPath    string

	CleanupExpiredCursors string
	GetCursor             string
	PutCursor             string

	GetValueIDsByItem string
	IncrementDocFreq  string
	DecrementDocFreq  string

	DeleteSearchRow      string
	DeletePresentByItem  string
	DeletePostingsByItem string
	DeleteNumberByItem   string
	DeleteDateByItem     string
	DeleteBoolByItem     string
	DeleteItemsByID      string

	InsertOrIgnoreKwDict    string
	GetKwDictID             string
	InsertOrIgnoreKwPosting string

	InsertFieldPresent string
	InsertFieldNumber  string
	InsertFieldDate    string
	InsertFieldBool    string

	UpsertItem       UpsertItemSQL
	UpsertItemWithTS UpsertItemSQL
}

// UpsertItemSQL handles item insertion/update
type UpsertItemSQL interface {
	Build(path string, dataJSON []byte, createdAtMS, updatedAtMS int64, nowMode bool) (string, []any)
}

// FTS handles full-text search operations
type FTS interface {
	HasFTS(schema Schema) bool
	CreateFTS(ctx context.Context, db *sql.DB, schema Schema) error
	VerifyFTS(ctx context.Context, db *sql.DB, schema Schema) error
	AddTextColumns(ctx context.Context, db *sql.DB, old, new Schema) error

	DeleteRow(ctx context.Context, tx *sql.Tx, itemID int64) error
	UpsertRow(ctx context.Context, tx *sql.Tx, itemID int64, schema Schema, textVals map[string]*string) error

	// CompileTextPredicate returns SQL body (without WITH name) that yields item_id
	CompileTextPredicate(b Builder, schema Schema, pred TextPredicate) (sql string, args []any, err error)

	// ScoreCTEsAndJoin returns extra CTEs, join SQL fragment, and a score expression
	// It may use builder to allocate placeholders
	ScoreCTEsAndJoin(b Builder, schema Schema, preds []TextPredicate) (extraCTEs []CTE, joinSQL string, scoreExpr string, err error)
}

// Builder interface for placeholder management
type Builder interface {
	Arg(v any) string
	Args() []any
	Len() int
}

// TextPredicate represents a text search predicate
type TextPredicate struct {
	Field *string
	Query string
}

// CTE represents a Common Table Expression
type CTE struct {
	Name string
	SQL  string
}
