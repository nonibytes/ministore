package index

import mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"

type BatchOpKind int

const (
	BatchPut BatchOpKind = iota
	BatchDelete
)

type BatchOp struct {
	Kind BatchOpKind
	Doc  map[string]any
	Path string
}

type Batch struct {
	Ops []BatchOp
}

func NewBatch() Batch { return Batch{Ops: []BatchOp{}} }

func (b *Batch) Put(doc map[string]any) error {
	b.Ops = append(b.Ops, BatchOp{Kind: BatchPut, Doc: doc})
	return nil
}

func (b *Batch) Delete(path string) error {
	b.Ops = append(b.Ops, BatchOp{Kind: BatchDelete, Path: path})
	return nil
}

func (b Batch) Len() int { return len(b.Ops) }

// ValidateBasic checks shape; adapters still validate schema.
func (b Batch) ValidateBasic() error {
	if len(b.Ops) == 0 {
		return nil
	}
	for _, op := range b.Ops {
		switch op.Kind {
		case BatchPut:
			if op.Doc == nil {
				return mserrors.NewError(mserrors.ErrSchema, "batch put doc is nil")
			}
		case BatchDelete:
			if op.Path == "" {
				return mserrors.NewError(mserrors.ErrSchema, "batch delete path is empty")
			}
		}
	}
	return nil
}
