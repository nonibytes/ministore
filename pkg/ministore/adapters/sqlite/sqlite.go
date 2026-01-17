package sqlite

import (
	"context"

	"github.com/nonibytes/ministore/pkg/ministore/backend"
	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
	"github.com/nonibytes/ministore/pkg/ministore/index"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
	"github.com/nonibytes/ministore/pkg/ministore/types"
)

type Options struct {
	// Path is either a directory (registry lists *.db) or an explicit .db file path.
	Path string
}

type Backend struct {
	opts Options
}

func NewBackend(opts Options) *Backend { return &Backend{opts: opts} }

func (b *Backend) Name() string { return "sqlite" }

func (b *Backend) Capabilities() backend.Capabilities {
	return backend.Capabilities{
		FullText:          true,
		KeywordExact:      true,
		KeywordPrefix:     true,
		KeywordContains:   true,
		KeywordGlob:       true,
		PathPrefix:        true,
		PathGlob:          true,
		HasPredicate:      true,
		NumberOps:         true,
		DateOps:           true,
		BoolOps:           true,
		RankDefault:       true,
		RankRecency:       true,
		RankField:         true,
		RankNone:          true,
		CursorShort:       true,
		CursorFull:        true,
		DiscoverValues:    true,
		StatsAggregations: true,
	}
}

func (b *Backend) Registry() backend.Registry {
	return &Registry{opts: b.opts}
}

func (b *Backend) OpenIndexStore(ctx context.Context, indexName string, sch schema.Schema, opts types.IndexOptions) (backend.IndexStore, error) {
	_ = ctx
	// TODO: open/create sqlite DB file at indexName (already resolved by CLI for sqlite)
	return &Store{dbPath: indexName, schema: sch, opts: opts}, nil
}

// ---- Registry ----

type Registry struct {
	opts Options
}

func (r *Registry) Create(ctx context.Context, name string, sch schema.Schema) error {
	_ = name
	_ = sch
	return mserrors.Wrap(mserrors.ErrNotImpl, "sqlite registry create", mserrors.ErrNotImplemented)
}

func (r *Registry) Get(ctx context.Context, name string) (schema.Schema, error) {
	_ = name
	return schema.Schema{}, mserrors.Wrap(mserrors.ErrNotImpl, "sqlite registry get", mserrors.ErrNotImplemented)
}

func (r *Registry) List(ctx context.Context) ([]backend.IndexInfo, error) {
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "sqlite registry list", mserrors.ErrNotImplemented)
}

func (r *Registry) Drop(ctx context.Context, name string) error {
	_ = name
	return mserrors.Wrap(mserrors.ErrNotImpl, "sqlite registry drop", mserrors.ErrNotImplemented)
}

// ---- Store ----

type Store struct {
	dbPath string
	schema schema.Schema
	opts   index.IndexOptions
}
