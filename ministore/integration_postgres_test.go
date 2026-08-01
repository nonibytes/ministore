package ministore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/postgres"
)

func TestWriteBatchPostgres(t *testing.T) {
	dsn := os.Getenv("MINISTORE_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("MINISTORE_POSTGRES_TEST_DSN is not set")
	}

	ctx := context.Background()
	schemaName := fmt.Sprintf("ministore_write_batch_%d", time.Now().UnixNano())
	ix, err := ministore.Create(ctx, postgres.New(dsn, schemaName), ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"tags": {Type: ministore.FieldKeyword, Multi: true},
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

	count, err := ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		if err := writer.PutJSON([]byte(`{"path":"/one","tags":["a"]}`)); err != nil {
			return err
		}
		if err := writer.PutJSON([]byte(`{"path":"/two","tags":["a","b"]}`)); err != nil {
			return err
		}
		return writer.Delete("/missing")
	})
	if err != nil || count != 2 {
		t.Fatalf("WriteBatch = (%d, %v), want (2, nil)", count, err)
	}

	want := errors.New("rollback")
	count, err = ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		if err := writer.Delete("/one"); err != nil {
			return err
		}
		if err := writer.PutJSON([]byte(`{"path":"/three","tags":["c"]}`)); err != nil {
			return err
		}
		return want
	})
	if count != 0 || !errors.Is(err, want) {
		t.Fatalf("rolled-back WriteBatch = (%d, %v)", count, err)
	}
	if _, err := ix.Get(ctx, "/one"); err != nil {
		t.Fatalf("Get(/one) after rollback: %v", err)
	}
	if _, err := ix.Get(ctx, "/three"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("Get(/three) error = %v, want not found", err)
	}

	var docFreq int
	if err := ix.DB().QueryRowContext(ctx, `SELECT doc_freq FROM "`+schemaName+`".kw_dict WHERE field = $1 AND value = $2`, "tags", "a").Scan(&docFreq); err != nil {
		t.Fatal(err)
	}
	if docFreq != 2 {
		t.Fatalf("doc_freq(a) = %d, want 2", docFreq)
	}
}

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
