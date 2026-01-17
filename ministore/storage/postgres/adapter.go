package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

type Adapter struct {
	DSN    string
	Schema string // used as dedicated schema via search_path
}

func New(dsn, schema string) *Adapter {
	return &Adapter{DSN: dsn, Schema: schema}
}

func (a *Adapter) Backend() storage.Backend { return storage.BackendPostgres }

func (a *Adapter) PlaceholderStyle() sqlbuilder.PlaceholderStyle { return sqlbuilder.PlaceholderDollar }

func (a *Adapter) IndexID() string { return "postgres:" + a.Schema }

func (a *Adapter) Close() error { return nil }

func (a *Adapter) SQL() storage.SQL { return SQLTemplates }

func (a *Adapter) FTS() storage.FTS { return FTS{} }

var schemaNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quoteIdent(ident string) string {
	// ident is validated to contain no quotes; safe to wrap
	return `"` + ident + `"`
}

func (a *Adapter) ensureSchema(ctx context.Context, db *sql.DB) error {
	if a.Schema == "" || !schemaNameRe.MatchString(a.Schema) {
		return fmt.Errorf("invalid postgres schema name %q (must match %s)", a.Schema, schemaNameRe.String())
	}
	_, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoteIdent(a.Schema))
	return err
}

func (a *Adapter) Connect(ctx context.Context) (*sql.DB, error) {
	// 1) Connect without search_path to ensure schema exists
	cfg0, err := pgx.ParseConfig(a.DSN)
	if err != nil {
		return nil, err
	}
	db0 := stdlib.OpenDB(*cfg0)
	if err := db0.PingContext(ctx); err != nil {
		_ = db0.Close()
		return nil, err
	}
	if err := a.ensureSchema(ctx, db0); err != nil {
		_ = db0.Close()
		return nil, err
	}
	_ = db0.Close()

	// 2) Connect with search_path pinned to the schema
	cfg, err := pgx.ParseConfig(a.DSN)
	if err != nil {
		return nil, err
	}
	// Include public as a fallback for built-ins; schema is first.
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = make(map[string]string)
	}
	cfg.RuntimeParams["search_path"] = fmt.Sprintf("%s,public", quoteIdent(a.Schema))

	db := stdlib.OpenDB(*cfg)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (a *Adapter) CreateIndex(ctx context.Context, db *sql.DB, schemaJSON []byte) error {
	// Base schema
	if _, err := db.ExecContext(ctx, ddlBase); err != nil {
		return err
	}

	sqlt := a.SQL()
	if _, err := db.ExecContext(ctx, sqlt.SetMeta, "ministore_magic", "ministore"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, sqlt.SetMeta, "ministore_version", "1"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, sqlt.SetMeta, "schema_json", string(schemaJSON)); err != nil {
		return err
	}

	// FTS (if any text fields)
	schema, err := parseSchema(schemaJSON)
	if err != nil {
		return err
	}
	if a.FTS().HasFTS(schema) {
		if err := a.FTS().CreateFTS(ctx, db, schema); err != nil {
			return err
		}
	}

	return nil
}

func (a *Adapter) OpenIndex(ctx context.Context, db *sql.DB) ([]byte, error) {
	sqlt := a.SQL()
	var magic string
	if err := db.QueryRowContext(ctx, sqlt.GetMeta, "ministore_magic").Scan(&magic); err != nil {
		return nil, err
	}
	if magic != "ministore" {
		return nil, fmt.Errorf("not a ministore db")
	}
	var schemaStr string
	if err := db.QueryRowContext(ctx, sqlt.GetMeta, "schema_json").Scan(&schemaStr); err != nil {
		return nil, err
	}
	return []byte(schemaStr), nil
}

func (a *Adapter) VerifyFTS(ctx context.Context, db *sql.DB, schema storage.Schema) error {
	if !a.FTS().HasFTS(schema) {
		return nil
	}
	return a.FTS().VerifyFTS(ctx, db, schema)
}

func (a *Adapter) ApplySchemaAdditive(ctx context.Context, db *sql.DB, old, new storage.Schema) error {
	if a.FTS().HasFTS(new) {
		if err := a.FTS().AddTextColumns(ctx, db, old, new); err != nil {
			return err
		}
	}
	b, err := new.ToJSON()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, a.SQL().SetMeta, "schema_json", string(b))
	return err
}

func (a *Adapter) Optimize(ctx context.Context, db *sql.DB) error {
	// Best-effort: ANALYZE
	_, _ = db.ExecContext(ctx, "ANALYZE")
	return nil
}

type fieldSpec struct {
	Type   string
	Multi  bool
	Weight *float64
}

func parseSchema(schemaJSON []byte) (storage.Schema, error) {
	var raw struct {
		Fields map[string]struct {
			Type   string   `json:"type"`
			Multi  bool     `json:"multi,omitempty"`
			Weight *float64 `json:"weight,omitempty"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(schemaJSON, &raw); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	fields := make(map[string]fieldSpec, len(raw.Fields))
	for name, spec := range raw.Fields {
		fields[name] = fieldSpec{Type: spec.Type, Multi: spec.Multi, Weight: spec.Weight}
	}
	return &parsedSchema{data: schemaJSON, fields: fields}, nil
}

type parsedSchema struct {
	data   []byte
	fields map[string]fieldSpec
}

func (s *parsedSchema) ToJSON() ([]byte, error) { return s.data, nil }

func (s *parsedSchema) TextFieldsInOrder() []storage.TextField {
	var names []string
	for name, spec := range s.fields {
		if spec.Type == "text" {
			names = append(names, name)
		}
	}
	// stable order
	sqlbuilder.SortStrings(names)

	out := make([]storage.TextField, 0, len(names))
	for _, name := range names {
		spec := s.fields[name]
		w := 1.0
		if spec.Weight != nil {
			w = *spec.Weight
		}
		out = append(out, storage.TextField{Name: name, Weight: w})
	}
	return out
}

func (s *parsedSchema) Get(name string) (storage.FieldSpec, bool) {
	spec, ok := s.fields[name]
	if !ok {
		return storage.FieldSpec{}, false
	}
	return storage.FieldSpec{
		Type:   storage.FieldType(spec.Type),
		Multi:  spec.Multi,
		Weight: spec.Weight,
	}, true
}

func (s *parsedSchema) HasField(name string) bool {
	_, ok := s.fields[name]
	return ok
}
