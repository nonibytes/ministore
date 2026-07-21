package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestVerifyFTSUsesDeclaredColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("CREATE VIRTUAL TABLE search USING fts5(body, title)"); err != nil {
		t.Fatal(err)
	}

	schema := &parsedSchema{fields: map[string]fieldSpec{
		"body":  {Type: "text"},
		"title": {Type: "text"},
	}}
	if err := (FTS5{}).VerifyFTS(context.Background(), db, schema); err != nil {
		t.Fatalf("VerifyFTS() error = %v", err)
	}
}

func TestVerifyFTSRejectsColumnMismatch(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec("CREATE VIRTUAL TABLE search USING fts5(title)"); err != nil {
		t.Fatal(err)
	}

	schema := &parsedSchema{fields: map[string]fieldSpec{
		"body":  {Type: "text"},
		"title": {Type: "text"},
	}}
	err = (FTS5{}).VerifyFTS(context.Background(), db, schema)
	if err == nil || !strings.Contains(err.Error(), "columns mismatch") {
		t.Fatalf("VerifyFTS() error = %v, want column mismatch", err)
	}
}

func TestVerifyFTSRejectsMissingTable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := &parsedSchema{fields: map[string]fieldSpec{
		"title": {Type: "text"},
	}}
	err = (FTS5{}).VerifyFTS(context.Background(), db, schema)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("VerifyFTS() error = %v, want missing table", err)
	}
}
