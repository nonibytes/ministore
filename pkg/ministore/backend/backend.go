package backend

import (
	"context"

	"github.com/nonibytes/ministore/pkg/ministore/cursor"
	"github.com/nonibytes/ministore/pkg/ministore/item"
	"github.com/nonibytes/ministore/pkg/ministore/plan"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
	"github.com/nonibytes/ministore/pkg/ministore/types"
)

type IndexInfo struct {
	Name        string `json:"name"`
	Backend     string `json:"backend"`
	CreatedAtMs int64  `json:"created_at_ms,omitempty"`
}

type ValueCount struct {
	Value string `json:"value"`
	Count uint64 `json:"count"`
}

type FieldOverviewRow struct {
	Field    string   `json:"field"`
	Type     string   `json:"type"`
	DocCount int64    `json:"doc_count"`
	Weight   *float64 `json:"weight,omitempty"`
}

type Backend interface {
	Capabilities() Capabilities
	Registry() Registry
	OpenIndexStore(ctx context.Context, indexName string, sch schema.Schema, opts types.IndexOptions) (IndexStore, error)
}

type Registry interface {
	Create(ctx context.Context, name string, sch schema.Schema) error
	Get(ctx context.Context, name string) (schema.Schema, error)
	List(ctx context.Context) ([]IndexInfo, error)
	Drop(ctx context.Context, name string) error
}

type IndexStore interface {
	Put(ctx context.Context, doc map[string]any) error
	Get(ctx context.Context, path string) (item.ItemView, error)
	Peek(ctx context.Context, path string) (map[string]any, error)
	Delete(ctx context.Context, path string) (bool, error)
	DeleteWhere(ctx context.Context, cq plan.CompiledQuery) (int, error)

	Search(ctx context.Context, cq plan.CompiledQuery, opts types.SearchOptions) (types.SearchResultPage, error)

	DiscoverValues(ctx context.Context, field string, scope *plan.CompiledQuery, top int) ([]ValueCount, error)
	DiscoverFields(ctx context.Context) ([]FieldOverviewRow, error)
	Stats(ctx context.Context, field string, scope *plan.CompiledQuery) (map[string]any, error)

	ApplySchemaAdditive(ctx context.Context, newSchema schema.Schema) error
	MigrateRebuild(ctx context.Context, newName string, newSchema schema.Schema) error

	Optimize(ctx context.Context) error
}

// Optional interface for short cursor lookup.
// Adapters that support short cursors should implement this.
type ShortCursorResolver interface {
	LoadShortCursor(ctx context.Context, handle string) (cursor.Position, error)
}
