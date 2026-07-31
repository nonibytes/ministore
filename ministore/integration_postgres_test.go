package ministore_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/postgres"
)

func TestScanPathsPostgres(t *testing.T) {
	dsn := os.Getenv("MINISTORE_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("MINISTORE_POSTGRES_TEST_DSN is not set")
	}

	ctx := context.Background()
	schemaName := fmt.Sprintf("ministore_scan_%d", time.Now().UnixNano())
	ix, err := ministore.Create(ctx, postgres.New(dsn, schemaName), ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"marker": {Type: ministore.FieldKeyword},
		},
	}, ministore.DefaultIndexOptions())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		if _, err := ix.DB().ExecContext(context.Background(), `DROP SCHEMA "`+schemaName+`" CASCADE`); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		if err := ix.Close(); err != nil {
			t.Errorf("close index: %v", err)
		}
	})

	for _, path := range []string{"/z", "/a_", "/aa", "/a%", "/a", "/Ω", "/é"} {
		if err := ix.PutJSON(ctx, []byte(`{"path":"`+path+`"}`)); err != nil {
			t.Fatalf("PutJSON(%q): %v", path, err)
		}
	}

	var got []string
	if err := ix.ScanPaths(ctx, "/a", func(path string) error {
		got = append(got, path)
		return nil
	}); err != nil {
		t.Fatalf("ScanPaths: %v", err)
	}
	want := []string{"/a", "/a%", "/a_", "/aa"}
	if !equalStrings(got, want) {
		t.Fatalf("ScanPaths = %q, want %q", got, want)
	}
}
