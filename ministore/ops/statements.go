package ops

import (
	"context"
	"database/sql"
	"fmt"
)

// StatementCache prepares each SQL statement at most once for a transaction.
// It is intentionally transaction-scoped because prepared statements created
// from sql.Tx cannot outlive that transaction.
type StatementCache struct {
	tx         *sql.Tx
	statements map[string]*sql.Stmt
}

func NewStatementCache(tx *sql.Tx) *StatementCache {
	return &StatementCache{
		tx:         tx,
		statements: make(map[string]*sql.Stmt),
	}
}

func (c *StatementCache) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	stmt, err := c.prepare(ctx, query)
	if err != nil {
		return nil, err
	}
	return stmt.ExecContext(ctx, args...)
}

func (c *StatementCache) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	stmt, err := c.prepare(ctx, query)
	if err != nil {
		return nil, err
	}
	return stmt.QueryContext(ctx, args...)
}

func (c *StatementCache) Close() error {
	var firstErr error
	for query, stmt := range c.statements {
		if err := stmt.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close prepared statement %q: %w", query, err)
		}
		delete(c.statements, query)
	}
	return firstErr
}

func (c *StatementCache) prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	if stmt, ok := c.statements[query]; ok {
		return stmt, nil
	}
	stmt, err := c.tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("prepare statement: %w", err)
	}
	c.statements[query] = stmt
	return stmt, nil
}

func scanOne(ctx context.Context, exec interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, query string, args []any, destinations ...any) error {
	rows, err := exec.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(destinations...); err != nil {
		return err
	}
	return rows.Err()
}
