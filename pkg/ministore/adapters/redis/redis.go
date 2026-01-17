package redis

import (
	"context"

	"github.com/nonibytes/ministore/pkg/ministore/backend"
	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
	"github.com/nonibytes/ministore/pkg/ministore/index"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
	"github.com/nonibytes/ministore/pkg/ministore/types"
)

type Options struct {
	Addr     string
	Password string
	DB       int
}

type Backend struct {
	opts Options
}

func NewBackend(opts Options) *Backend { return &Backend{opts: opts} }

func (b *Backend) Name() string { return "redis" }

func (b *Backend) Capabilities() backend.Capabilities {
	// NOTE: This assumes RediSearch module is available.
	return backend.Capabilities{
		FullText:          true,
		KeywordExact:      true,
		KeywordPrefix:     true,
		KeywordContains:   true, // via TAGVALS expansion
		KeywordGlob:       true, // via TAGVALS expansion
		PathPrefix:        true,
		PathGlob:          true, // via prefix + post-filter
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
	return &Store{indexName: indexName, schema: sch, opts: opts}, nil
}

type Registry struct{ opts Options }

func (r *Registry) Create(ctx context.Context, name string, sch schema.Schema) error {
	_ = name
	_ = sch
	return mserrors.Wrap(mserrors.ErrNotImpl, "redis registry create", mserrors.ErrNotImplemented)
}
func (r *Registry) Get(ctx context.Context, name string) (schema.Schema, error) {
	_ = name
	return schema.Schema{}, mserrors.Wrap(mserrors.ErrNotImpl, "redis registry get", mserrors.ErrNotImplemented)
}
func (r *Registry) List(ctx context.Context) ([]backend.IndexInfo, error) {
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "redis registry list", mserrors.ErrNotImplemented)
}
func (r *Registry) Drop(ctx context.Context, name string) error {
	_ = name
	return mserrors.Wrap(mserrors.ErrNotImpl, "redis registry drop", mserrors.ErrNotImplemented)
}

type Store struct {
	indexName string
	schema    schema.Schema
	opts      index.IndexOptions
}
