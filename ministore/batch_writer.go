package ministore

import (
	"context"
	"database/sql"

	"github.com/ministore/ministore/ministore/ops"
	"github.com/ministore/ministore/ministore/storage"
)

// BatchWriter applies operations immediately inside a WriteBatch transaction.
// It is valid only during its callback and must not be used concurrently.
type BatchWriter struct {
	ctx        context.Context
	tx         *sql.Tx
	statements *ops.StatementCache
	puts       *ops.PutExecutor
	sql        storage.SQL
	fts        storage.FTS
	schema     storage.Schema
	nowMS      int64
	count      int
	firstErr   error
	active     bool
}

// WriteBatch runs write in one transaction and commits only if every writer
// operation and the callback succeed. The callback must not retain the writer.
func (ix *Index) WriteBatch(ctx context.Context, write func(*BatchWriter) error) (int, error) {
	if write == nil {
		return 0, New(ErrSchema, "batch callback cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return 0, Wrap(ErrSQL, "batch context", err)
	}

	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, Wrap(ErrSQL, "begin transaction", err)
	}
	defer tx.Rollback()

	sqlt := ix.adapter.SQL()
	fts := ix.adapter.FTS()
	statements := ops.NewStatementCache(tx)
	defer statements.Close()

	writer := &BatchWriter{
		ctx:        ctx,
		tx:         tx,
		statements: statements,
		puts:       ops.NewPutExecutor(statements, sqlt, fts, ix.schema.AsStorageSchema()),
		sql:        sqlt,
		fts:        fts,
		schema:     ix.schema.AsStorageSchema(),
		nowMS:      ix.nowMS(),
		active:     true,
	}
	callbackErr := write(writer)
	writer.active = false

	if callbackErr != nil {
		return 0, callbackErr
	}
	if writer.firstErr != nil {
		return 0, writer.firstErr
	}
	if err := ctx.Err(); err != nil {
		return 0, Wrap(ErrSQL, "batch context", err)
	}
	if err := writer.puts.FlushWorkingSet(ctx); err != nil {
		return 0, Wrap(ErrSQL, "flush batch working set", err)
	}
	if err := statements.Close(); err != nil {
		return 0, Wrap(ErrSQL, "close prepared statements", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, Wrap(ErrSQL, "commit transaction", err)
	}
	return writer.count, nil
}

// PutJSON inserts or replaces one document in the current transaction.
func (w *BatchWriter) PutJSON(doc []byte) error {
	if err := w.ready(); err != nil {
		return err
	}
	prep, err := ops.PreparePut(w.schema, doc)
	if err != nil {
		return w.fail(Wrap(ErrSchema, "prepare put", err))
	}
	return w.putPrepared(prep)
}

func (w *BatchWriter) putDocument(doc map[string]any, dataJSON []byte) error {
	if err := w.ready(); err != nil {
		return err
	}
	prep, err := ops.PreparePutDocument(w.schema, doc, dataJSON)
	if err != nil {
		return w.fail(Wrap(ErrSchema, "prepare put", err))
	}
	return w.putPrepared(prep)
}

func (w *BatchWriter) putPrepared(prep *ops.PutPrepared) error {
	if err := w.ready(); err != nil {
		return err
	}
	if _, _, err := w.puts.Execute(w.ctx, prep, w.nowMS); err != nil {
		return w.fail(Wrap(ErrSQL, "execute put", err))
	}
	if err := w.puts.FlushWorkingSet(w.ctx); err != nil {
		return w.fail(Wrap(ErrSQL, "flush batch working set", err))
	}
	w.count++
	return nil
}

// Delete removes path in the current transaction. A missing path is a no-op.
func (w *BatchWriter) Delete(path string) error {
	if err := w.ready(); err != nil {
		return err
	}
	if path == "" {
		return w.fail(New(ErrSchema, "path cannot be empty"))
	}
	if err := w.puts.FlushWorkingSet(w.ctx); err != nil {
		return w.fail(Wrap(ErrSQL, "flush batch working set", err))
	}

	var itemID int64
	var createdAt int64
	err := w.tx.QueryRowContext(w.ctx, w.sql.FindItemIDByPath, path).Scan(&itemID, &createdAt)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return w.fail(Wrap(ErrSQL, "find item", err))
	}
	if err := ops.DeleteByItemIDWithSchema(w.ctx, w.statements, w.sql, w.fts, w.schema, itemID); err != nil {
		return w.fail(Wrap(ErrSQL, "delete item", err))
	}
	w.count++
	return nil
}

func (w *BatchWriter) ready() error {
	if !w.active {
		return New(ErrSchema, "batch writer is no longer active")
	}
	if w.firstErr != nil {
		return w.firstErr
	}
	if err := w.ctx.Err(); err != nil {
		return w.fail(Wrap(ErrSQL, "batch context", err))
	}
	return nil
}

func (w *BatchWriter) fail(err error) error {
	if w.firstErr == nil {
		w.firstErr = err
	}
	return err
}
