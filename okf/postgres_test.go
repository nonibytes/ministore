package okf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/postgres"
)

func TestSyncPostgres(t *testing.T) {
	dsn := os.Getenv("MINISTORE_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("MINISTORE_POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("okf_test_%d", time.Now().UnixNano())
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	defer connection.Exec(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
	ix, err := ministore.Create(ctx, postgres.New(dsn, schema), ProjectionSchema(), ministore.DefaultIndexOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	bundle := t.TempDir()
	if err = os.WriteFile(filepath.Join(bundle, "one.md"), []byte("---\ntype: Note\n---\nBody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	first, err := Sync(ctx, bundle, ix, SyncOptions{})
	if err != nil || first.Added != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := Sync(ctx, bundle, ix, SyncOptions{})
	if err != nil || second.Unchanged != 1 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if _, err = ix.Get(ctx, "/one"); err != nil {
		t.Fatal(err)
	}
}
