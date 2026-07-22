package ops

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestStatementCacheReusesStatements(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	cache := NewStatementCache(tx)
	ctx := context.Background()
	if _, err := cache.ExecContext(ctx, "CREATE TABLE values_table(value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int{1, 2} {
		if _, err := cache.ExecContext(ctx, "INSERT INTO values_table(value) VALUES(?)", value); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		rows, err := cache.QueryContext(ctx, "SELECT value FROM values_table ORDER BY value")
		if err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if got := len(cache.statements); got != 3 {
		t.Fatalf("prepared statements = %d, want 3", got)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(cache.statements); got != 0 {
		t.Fatalf("prepared statements after Close = %d, want 0", got)
	}
}
