package ministore

import (
	"context"
	"encoding/json"
)

type BatchOpKind int

const (
	batchPut BatchOpKind = iota
	batchDelete
)

type BatchOp struct {
	Kind BatchOpKind
	Doc  []byte // for put
	Path string // for delete
}

type Batch struct {
	ops []BatchOp
}

func NewBatch() Batch {
	return Batch{ops: make([]BatchOp, 0)}
}

func (b *Batch) PutJSON(doc []byte) error {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return Wrap(ErrSchema, "document json", err)
	}
	p, ok := m["path"].(string)
	if !ok || p == "" {
		return New(ErrSchema, "document must contain non-empty 'path'")
	}
	b.ops = append(b.ops, BatchOp{Kind: batchPut, Doc: doc})
	return nil
}

func (b *Batch) Delete(path string) error {
	if path == "" {
		return New(ErrSchema, "path cannot be empty")
	}
	b.ops = append(b.ops, BatchOp{Kind: batchDelete, Path: path})
	return nil
}

func (b *Batch) Len() int {
	return len(b.ops)
}

func (b *Batch) Empty() bool {
	return len(b.ops) == 0
}

// Execute is implemented on Index to keep storage access internal
func (b *Batch) Execute(ctx context.Context, ix *Index) (int, error) {
	return ix.Batch(ctx, *b)
}
