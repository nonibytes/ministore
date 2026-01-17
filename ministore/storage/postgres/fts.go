package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ministore/ministore/ministore/storage"
)

type FTS struct{}

func (f FTS) HasFTS(schema storage.Schema) bool {
	return len(schema.TextFieldsInOrder()) > 0
}

func (f FTS) CreateFTS(ctx context.Context, db *sql.DB, schema storage.Schema) error {
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return nil
	}

	var cols []string
	cols = append(cols, "item_id BIGINT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE")
	for _, tf := range fields {
		cols = append(cols, fmt.Sprintf("%s TSVECTOR NOT NULL DEFAULT ''::tsvector", tf.Name))
	}

	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS search (%s)", strings.Join(cols, ", "))
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("create search table: %w", err)
	}

	for _, tf := range fields {
		idx := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_search_%s ON search USING GIN(%s)", tf.Name, tf.Name)
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("create gin index for %s: %w", tf.Name, err)
		}
	}

	return nil
}

func (f FTS) VerifyFTS(ctx context.Context, db *sql.DB, schema storage.Schema) error {
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return nil
	}

	// Existence check
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM search WHERE 0=1").Scan(&n); err != nil {
		return fmt.Errorf("FTS table verification failed: %w", err)
	}

	// Column check
	for _, tf := range fields {
		q := fmt.Sprintf("SELECT %s FROM search WHERE 0=1", tf.Name)
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("FTS column '%s' not found or invalid: %w", tf.Name, err)
		}
		rows.Close()
	}

	return nil
}

func (f FTS) AddTextColumns(ctx context.Context, db *sql.DB, old, new storage.Schema) error {
	oldFields := map[string]bool{}
	for _, tf := range old.TextFieldsInOrder() {
		oldFields[tf.Name] = true
	}

	// If old had no FTS but new does, create full FTS table.
	if len(old.TextFieldsInOrder()) == 0 && len(new.TextFieldsInOrder()) > 0 {
		return f.CreateFTS(ctx, db, new)
	}

	for _, tf := range new.TextFieldsInOrder() {
		if oldFields[tf.Name] {
			continue
		}
		alter := fmt.Sprintf("ALTER TABLE search ADD COLUMN %s TSVECTOR NOT NULL DEFAULT ''::tsvector", tf.Name)
		if _, err := db.ExecContext(ctx, alter); err != nil {
			return fmt.Errorf("alter search add column %s: %w", tf.Name, err)
		}
		idx := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_search_%s ON search USING GIN(%s)", tf.Name, tf.Name)
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("create gin index for %s: %w", tf.Name, err)
		}
	}
	return nil
}

func (f FTS) DeleteRow(ctx context.Context, tx *sql.Tx, itemID int64) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM search WHERE item_id = $1", itemID)
	// If no FTS table exists, treat as no-op.
	if err != nil && strings.Contains(err.Error(), "relation \"search\" does not exist") {
		return nil
	}
	return err
}

func (f FTS) UpsertRow(ctx context.Context, tx *sql.Tx, itemID int64, schema storage.Schema, textVals map[string]*string) error {
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return nil
	}

	cols := make([]string, 0, len(fields)+1)
	vals := make([]string, 0, len(fields)+1)
	sets := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)

	cols = append(cols, "item_id")
	vals = append(vals, "$1")
	args = append(args, itemID)

	for i, tf := range fields {
		cols = append(cols, tf.Name)
		ph := fmt.Sprintf("$%d", i+2)
		// Compute vector inside SQL
		vals = append(vals, fmt.Sprintf("to_tsvector('simple', %s)", ph))
		sets = append(sets, fmt.Sprintf("%s = EXCLUDED.%s", tf.Name, tf.Name))

		v := textVals[tf.Name]
		if v == nil {
			args = append(args, "")
		} else {
			args = append(args, *v)
		}
	}

	sqlStmt := fmt.Sprintf(
		"INSERT INTO search(%s) VALUES(%s) ON CONFLICT(item_id) DO UPDATE SET %s",
		strings.Join(cols, ", "),
		strings.Join(vals, ", "),
		strings.Join(sets, ", "),
	)

	_, err := tx.ExecContext(ctx, sqlStmt, args...)
	if err != nil && strings.Contains(err.Error(), "relation \"search\" does not exist") {
		return nil
	}
	return err
}

func (f FTS) CompileTextPredicate(b storage.Builder, schema storage.Schema, pred storage.TextPredicate) (string, []any, error) {
	tsq := tsQueryExpr(b, pred.Query)
	cond, err := matchCond(schema, pred, tsq)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("SELECT item_id FROM search WHERE %s", cond), nil, nil
}

func (f FTS) ScoreCTEsAndJoin(b storage.Builder, schema storage.Schema, preds []storage.TextPredicate) ([]storage.CTE, string, string, error) {
	if len(preds) == 0 {
		return nil, "", "NULL", nil
	}

	weights := map[string]float64{}
	for _, tf := range schema.TextFieldsInOrder() {
		weights[tf.Name] = tf.Weight
	}

	var ctes []storage.CTE
	var joins []string
	var scoreParts []string

	for i, p := range preds {
		name := fmt.Sprintf("fts_score_%d", i)
		tsq := tsQueryExpr(b, p.Query)
		cond, err := matchCond(schema, p, tsq)
		if err != nil {
			return nil, "", "", err
		}

		scoreExpr, err := rankExpr(schema, weights, p, tsq)
		if err != nil {
			return nil, "", "", err
		}

		ctes = append(ctes, storage.CTE{
			Name: name,
			SQL:  fmt.Sprintf("SELECT item_id, (%s) AS score FROM search WHERE %s", scoreExpr, cond),
		})
		joins = append(joins, fmt.Sprintf("LEFT JOIN %s ON %s.item_id = i.id", name, name))
		scoreParts = append(scoreParts, fmt.Sprintf("COALESCE(%s.score, 0)", name))
	}

	return ctes, strings.Join(joins, "\n  "), strings.Join(scoreParts, " + "), nil
}

func tsQueryExpr(b storage.Builder, q string) string {
	ph := b.Arg(q)
	// Phrase queries if whitespace, otherwise plain.
	if strings.IndexFunc(q, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) >= 0 {
		return fmt.Sprintf("phraseto_tsquery('simple', %s)", ph)
	}
	return fmt.Sprintf("plainto_tsquery('simple', %s)", ph)
}

func matchCond(schema storage.Schema, pred storage.TextPredicate, tsq string) (string, error) {
	if pred.Field != nil {
		spec, ok := schema.Get(*pred.Field)
		if !ok {
			return "", fmt.Errorf("unknown field: %s", *pred.Field)
		}
		if spec.Type != storage.FieldType("text") {
			return "", fmt.Errorf("FTS predicate used on non-text field %s", *pred.Field)
		}
		return fmt.Sprintf("search.%s @@ %s", *pred.Field, tsq), nil
	}

	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return "", fmt.Errorf("no text fields in schema for bare text query")
	}
	parts := make([]string, 0, len(fields))
	for _, tf := range fields {
		parts = append(parts, fmt.Sprintf("search.%s @@ %s", tf.Name, tsq))
	}
	return fmt.Sprintf("(%s)", strings.Join(parts, " OR ")), nil
}

func rankExpr(schema storage.Schema, weights map[string]float64, pred storage.TextPredicate, tsq string) (string, error) {
	if pred.Field != nil {
		w := weights[*pred.Field]
		return fmt.Sprintf("(%g * ts_rank_cd(search.%s, %s))", w, *pred.Field, tsq), nil
	}
	fields := schema.TextFieldsInOrder()
	if len(fields) == 0 {
		return "0", nil
	}
	parts := make([]string, 0, len(fields))
	for _, tf := range fields {
		w := weights[tf.Name]
		parts = append(parts, fmt.Sprintf("(%g * ts_rank_cd(search.%s, %s))", w, tf.Name, tsq))
	}
	return strings.Join(parts, " + "), nil
}
