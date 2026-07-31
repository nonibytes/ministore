package ministore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ministore/ministore/ministore"
)

func writeBatchTestSchema() ministore.Schema {
	return ministore.Schema{Fields: map[string]ministore.FieldSpec{
		"marker": {Type: ministore.FieldKeyword},
	}}
}

func TestWriteBatchStreamsMixedOperationsAtomically(t *testing.T) {
	ix, _ := newIndex(t, ministore.Schema{Fields: map[string]ministore.FieldSpec{
		"tags": {Type: ministore.FieldKeyword, Multi: true},
	}})
	ctx := context.Background()
	if err := ix.PutJSON(ctx, []byte(`{"path":"/delete","tags":["old"]}`)); err != nil {
		t.Fatal(err)
	}

	count, err := ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		if err := writer.PutJSON([]byte(`{"path":"/one","tags":["a"]}`)); err != nil {
			return err
		}
		if err := writer.PutJSON([]byte(`{"path":"/one","tags":["b"]}`)); err != nil {
			return err
		}
		if err := writer.Delete("/delete"); err != nil {
			return err
		}
		return writer.Delete("/missing")
	})
	if err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if _, err := ix.Get(ctx, "/delete"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("deleted item error = %v, want not found", err)
	}
	item, err := ix.Get(ctx, "/one")
	if err != nil {
		t.Fatal(err)
	}
	if string(item.DocJSON) != `{"path":"/one","tags":["b"]}` {
		t.Fatalf("stored item = %s", item.DocJSON)
	}

	var a, b int
	if err := ix.DB().QueryRowContext(ctx, "SELECT doc_freq FROM kw_dict WHERE field = ? AND value = ?", "tags", "a").Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := ix.DB().QueryRowContext(ctx, "SELECT doc_freq FROM kw_dict WHERE field = ? AND value = ?", "tags", "b").Scan(&b); err != nil {
		t.Fatal(err)
	}
	if a != 0 || b != 1 {
		t.Fatalf("document frequencies = a:%d b:%d, want a:0 b:1", a, b)
	}
}

func TestWriteBatchCallbackFailureRollsBackAndPreservesCause(t *testing.T) {
	ix, _ := newIndex(t, writeBatchTestSchema())
	ctx := context.Background()
	want := errors.New("stop")
	count, err := ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		if err := writer.PutJSON([]byte(`{"path":"/rolled-back"}`)); err != nil {
			return err
		}
		return fmt.Errorf("callback: %w", want)
	})
	if count != 0 || !errors.Is(err, want) {
		t.Fatalf("WriteBatch = (%d, %v), want (0, error wrapping stop)", count, err)
	}
	if _, err := ix.Get(ctx, "/rolled-back"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("rolled-back item error = %v, want not found", err)
	}
}

func TestWriteBatchOperationFailurePoisonsTransaction(t *testing.T) {
	ix, _ := newIndex(t, writeBatchTestSchema())
	ctx := context.Background()
	count, err := ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		if err := writer.PutJSON([]byte(`{"path":"/rolled-back"}`)); err != nil {
			return err
		}
		_ = writer.PutJSON([]byte(`{"missing":"path"}`))
		return nil
	})
	if count != 0 || err == nil || !ministore.IsKind(err, ministore.ErrSchema) {
		t.Fatalf("WriteBatch = (%d, %v), want poisoned schema error", count, err)
	}
	if _, err := ix.Get(ctx, "/rolled-back"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("rolled-back item error = %v, want not found", err)
	}
}

func TestWriteBatchCancellationBeforeCommitRollsBack(t *testing.T) {
	ix, _ := newIndex(t, writeBatchTestSchema())
	ctx, cancel := context.WithCancel(context.Background())
	count, err := ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		if err := writer.PutJSON([]byte(`{"path":"/rolled-back"}`)); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if count != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteBatch = (%d, %v), want context cancellation", count, err)
	}
	if _, err := ix.Get(context.Background(), "/rolled-back"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("rolled-back item error = %v, want not found", err)
	}
}

func TestWriteBatchWriterExpiresWithCallback(t *testing.T) {
	ix, _ := newIndex(t, writeBatchTestSchema())
	ctx := context.Background()
	var saved *ministore.BatchWriter
	if _, err := ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		saved = writer
		return writer.PutJSON([]byte(`{"path":"/committed"}`))
	}); err != nil {
		t.Fatal(err)
	}
	if err := saved.PutJSON([]byte(`{"path":"/late"}`)); err == nil {
		t.Fatal("PutJSON after callback succeeded")
	}
	if _, err := ix.Get(ctx, "/late"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("late item error = %v, want not found", err)
	}
}

func TestWriteBatchStreamsLargeInput(t *testing.T) {
	ix, _ := newIndex(t, writeBatchTestSchema())
	const documents = 2000
	count, err := ix.WriteBatch(context.Background(), func(writer *ministore.BatchWriter) error {
		for i := 0; i < documents; i++ {
			if err := writer.PutJSON([]byte(fmt.Sprintf(`{"path":"/%04d"}`, i))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || count != documents {
		t.Fatalf("WriteBatch = (%d, %v), want (%d, nil)", count, err, documents)
	}
	var got int
	if err := ix.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM items").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != documents {
		t.Fatalf("count after batch = %d, want %d", got, documents)
	}
}
