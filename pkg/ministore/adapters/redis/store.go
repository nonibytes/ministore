package redis

import (
	"context"

	"github.com/nonibytes/ministore/pkg/ministore/backend"
	"github.com/nonibytes/ministore/pkg/ministore/cursor"
	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
	"github.com/nonibytes/ministore/pkg/ministore/index"
	"github.com/nonibytes/ministore/pkg/ministore/item"
	"github.com/nonibytes/ministore/pkg/ministore/plan"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
)

func (s *Store) Put(ctx context.Context, doc map[string]any) error {
	_ = ctx
	_ = doc
	return mserrors.Wrap(mserrors.ErrNotImpl, "redis put", mserrors.ErrNotImplemented)
}

func (s *Store) Get(ctx context.Context, path string) (item.ItemView, error) {
	_ = ctx
	_ = path
	return item.ItemView{}, mserrors.Wrap(mserrors.ErrNotImpl, "redis get", mserrors.ErrNotImplemented)
}

func (s *Store) Peek(ctx context.Context, path string) (map[string]any, error) {
	_ = ctx
	_ = path
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "redis peek", mserrors.ErrNotImplemented)
}

func (s *Store) Delete(ctx context.Context, path string) (bool, error) {
	_ = ctx
	_ = path
	return false, mserrors.Wrap(mserrors.ErrNotImpl, "redis delete", mserrors.ErrNotImplemented)
}

func (s *Store) DeleteWhere(ctx context.Context, cq plan.CompiledQuery) (int, error) {
	_ = ctx
	_ = cq
	return 0, mserrors.Wrap(mserrors.ErrNotImpl, "redis delete where", mserrors.ErrNotImplemented)
}

func (s *Store) Search(ctx context.Context, cq plan.CompiledQuery, opts index.SearchOptions) (index.SearchResultPage, error) {
	_ = ctx
	_ = cq
	_ = opts
	return index.SearchResultPage{}, mserrors.Wrap(mserrors.ErrNotImpl, "redis search", mserrors.ErrNotImplemented)
}

func (s *Store) DiscoverValues(ctx context.Context, field string, scope *plan.CompiledQuery, top int) ([]backend.ValueCount, error) {
	_ = ctx
	_ = field
	_ = scope
	_ = top
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "redis discover values", mserrors.ErrNotImplemented)
}

func (s *Store) DiscoverFields(ctx context.Context) ([]backend.FieldOverviewRow, error) {
	_ = ctx
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "redis discover fields", mserrors.ErrNotImplemented)
}

func (s *Store) Stats(ctx context.Context, field string, scope *plan.CompiledQuery) (map[string]any, error) {
	_ = ctx
	_ = field
	_ = scope
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "redis stats", mserrors.ErrNotImplemented)
}

func (s *Store) ApplySchemaAdditive(ctx context.Context, newSchema schema.Schema) error {
	_ = ctx
	_ = newSchema
	return mserrors.Wrap(mserrors.ErrNotImpl, "redis apply schema", mserrors.ErrNotImplemented)
}

func (s *Store) MigrateRebuild(ctx context.Context, newName string, newSchema schema.Schema) error {
	_ = ctx
	_ = newName
	_ = newSchema
	return mserrors.Wrap(mserrors.ErrNotImpl, "redis migrate", mserrors.ErrNotImplemented)
}

func (s *Store) Optimize(ctx context.Context) error {
	_ = ctx
	return mserrors.Wrap(mserrors.ErrNotImpl, "redis optimize", mserrors.ErrNotImplemented)
}

func (s *Store) LoadShortCursor(ctx context.Context, handle string) (cursor.Position, error) {
	_ = ctx
	_ = handle
	return cursor.Position{}, mserrors.Wrap(mserrors.ErrNotImpl, "redis cursor load", mserrors.ErrNotImplemented)
}
