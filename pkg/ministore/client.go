package ministore

import (
	"context"

	"github.com/nonibytes/ministore/pkg/ministore/backend"
	"github.com/nonibytes/ministore/pkg/ministore/index"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
)

type Client struct {
	backend backend.Backend
}

func NewClient(b backend.Backend) *Client { return &Client{backend: b} }

func (c *Client) CreateIndex(ctx context.Context, name string, sch schema.Schema, opts index.IndexOptions) (*index.Index, error) {
	if err := sch.Validate(); err != nil {
		return nil, err
	}
	if err := c.backend.Registry().Create(ctx, name, sch); err != nil {
		return nil, err
	}
	store, err := c.backend.OpenIndexStore(ctx, name, sch, opts)
	if err != nil {
		return nil, err
	}
	return index.New(name, sch, store, opts), nil
}

func (c *Client) OpenIndex(ctx context.Context, name string, opts index.IndexOptions) (*index.Index, error) {
	sch, err := c.backend.Registry().Get(ctx, name)
	if err != nil {
		return nil, err
	}
	store, err := c.backend.OpenIndexStore(ctx, name, sch, opts)
	if err != nil {
		return nil, err
	}
	return index.New(name, sch, store, opts), nil
}

func (c *Client) ListIndexes(ctx context.Context) ([]backend.IndexInfo, error) {
	return c.backend.Registry().List(ctx)
}

func (c *Client) DropIndex(ctx context.Context, name string) error {
	return c.backend.Registry().Drop(ctx, name)
}
