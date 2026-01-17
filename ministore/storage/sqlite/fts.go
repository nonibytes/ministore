package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ministore/ministore/ministore/storage"
)

type FTS5 struct{}

func (f FTS5) HasFTS(schema storage.Schema) bool {
	return len(schema.TextFieldsInOrder()) > 0
}

func (f FTS5) CreateFTS(ctx context.Context, db *sql.DB, schema storage.Schema) error {
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields))
	for _, tf := range fields {
		cols = append(cols, tf.Name)
	}
	sqlStmt := fmt.Sprintf("CREATE VIRTUAL TABLE IF NOT EXISTS search USING fts5(%s, tokenize='unicode61')", strings.Join(cols, ", "))
	_, err := db.ExecContext(ctx, sqlStmt)
	if err != nil {
		return fmt.Errorf("create fts: %w", err)
	}
	return nil
}

func (f FTS5) VerifyFTS(ctx context.Context, db *sql.DB, schema storage.Schema) error {
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return nil
	}

	// For FTS5 virtual tables, we need to query the table itself to verify columns
	// PRAGMA table_info doesn't work reliably for virtual tables
	// Instead, we'll try a simple query to verify the table exists and has the right structure
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM search WHERE 0=1").Scan(&count)
	if err != nil {
		// Table doesn't exist or is malformed
		if strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("FTS table 'search' does not exist")
		}
		return fmt.Errorf("FTS table verification failed: %w", err)
	}

	// Verify we can query each expected column
	for _, tf := range fields {
		testQuery := fmt.Sprintf("SELECT %s FROM search WHERE 0=1", tf.Name)
		_, err := db.QueryContext(ctx, testQuery)
		if err != nil {
			return fmt.Errorf("FTS column '%s' not found or invalid: %w", tf.Name, err)
		}
	}

	return nil
}

func (f FTS5) AddTextColumns(ctx context.Context, db *sql.DB, old, new storage.Schema) error {
	oldFields := map[string]bool{}
	for _, tf := range old.TextFieldsInOrder() {
		oldFields[tf.Name] = true
	}
	for _, tf := range new.TextFieldsInOrder() {
		if !oldFields[tf.Name] {
			_, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE search ADD COLUMN %s", tf.Name))
			if err != nil {
				return fmt.Errorf("alter fts: %w", err)
			}
		}
	}
	return nil
}

func (f FTS5) DeleteRow(ctx context.Context, tx *sql.Tx, itemID int64) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM search WHERE rowid = ?", itemID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table: search") {
			return nil
		}
		return fmt.Errorf("delete fts row: %w", err)
	}
	return nil
}

func (f FTS5) UpsertRow(ctx context.Context, tx *sql.Tx, itemID int64, schema storage.Schema, textVals map[string]*string) error {
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields)+1)
	cols = append(cols, "rowid")
	placeholders := make([]string, 0, len(fields)+1)
	placeholders = append(placeholders, "?")
	args := make([]any, 0, len(fields)+1)
	args = append(args, itemID)
	for _, tf := range fields {
		cols = append(cols, tf.Name)
		placeholders = append(placeholders, "?")
		v := textVals[tf.Name]
		if v == nil {
			args = append(args, nil)
		} else {
			args = append(args, *v)
		}
	}
	sqlStmt := fmt.Sprintf("INSERT INTO search(%s) VALUES(%s)", strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	_, err := tx.ExecContext(ctx, sqlStmt, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table: search") {
			return nil
		}
		return fmt.Errorf("insert fts row: %w", err)
	}
	return nil
}

func (f FTS5) CompileTextPredicate(b storage.Builder, schema storage.Schema, pred storage.TextPredicate) (string, []any, error) {
	match := buildMatchString(schema, pred)
	ph := b.Arg(match)
	return fmt.Sprintf("SELECT rowid AS item_id FROM search WHERE search MATCH %s", ph), nil, nil
}

func (f FTS5) ScoreCTEsAndJoin(b storage.Builder, schema storage.Schema, preds []storage.TextPredicate) ([]storage.CTE, string, string, error) {
	if len(preds) == 0 {
		return nil, "", "NULL", nil
	}
	parts := make([]string, 0, len(preds))
	for _, p := range preds {
		parts = append(parts, buildMatchString(schema, p))
	}
	match := strings.Join(parts, " AND ")
	ph := b.Arg(match)
	weights := schema.TextFieldsInOrder()
	wparts := make([]string, 0, len(weights))
	for _, tf := range weights {
		wparts = append(wparts, fmt.Sprintf("%g", tf.Weight))
	}
	wstr := strings.Join(wparts, ", ")
	cte := storage.CTE{
		Name: "fts_score",
		SQL:  fmt.Sprintf("SELECT rowid AS item_id, (-bm25(search, %s)) AS score FROM search WHERE search MATCH %s", wstr, ph),
	}
	joinSQL := "LEFT JOIN fts_score ON fts_score.item_id = i.id"
	scoreExpr := "COALESCE(fts_score.score, 0)"
	return []storage.CTE{cte}, joinSQL, scoreExpr, nil
}

func buildMatchString(schema storage.Schema, pred storage.TextPredicate) string {
	term := quoteFTSTerm(pred.Query)
	if pred.Field != nil {
		return fmt.Sprintf("%s:%s", *pred.Field, term)
	}
	fields := schema.TextFieldsInOrder()
	parts := make([]string, 0, len(fields))
	for _, tf := range fields {
		parts = append(parts, fmt.Sprintf("%s:%s", tf.Name, term))
	}
	return fmt.Sprintf("(%s)", strings.Join(parts, " OR "))
}

func quoteFTSTerm(term string) string {
	need := false
	for _, c := range term {
		switch {
		case c == '"' || c == ':' || c == '*' || c == '(' || c == ')' || c == '^':
			need = true
		case c <= ' ':
			need = true
		}
		if need {
			break
		}
	}
	if !need {
		return term
	}
	esc := strings.ReplaceAll(term, "\"", "\"\"")
	return fmt.Sprintf("\"%s\"", esc)
}
