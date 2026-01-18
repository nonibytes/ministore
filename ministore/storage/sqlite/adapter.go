package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

type Adapter struct {
	Path       string
	DriverName string
}

func New(path string) *Adapter {
	return &Adapter{Path: path, DriverName: "sqlite"}
}

func NewWithDriver(path, driver string) *Adapter {
	return &Adapter{Path: path, DriverName: driver}
}

func (a *Adapter) Backend() storage.Backend {
	return storage.BackendSQLite
}

func (a *Adapter) PlaceholderStyle() sqlbuilder.PlaceholderStyle {
	return sqlbuilder.PlaceholderQuestion
}

func (a *Adapter) IndexID() string {
	return a.Path
}

func (a *Adapter) Connect(ctx context.Context) (*sql.DB, error) {
	dsn := a.Path
	if !strings.Contains(dsn, "?") {
		dsn = dsn + "?_busy_timeout=5000&_foreign_keys=on"
	} else {
		dsn = dsn + "&_busy_timeout=5000&_foreign_keys=on"
	}
	db, err := sql.Open(a.DriverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func (a *Adapter) Close() error {
	return nil
}

func (a *Adapter) SQL() storage.SQL {
	return SQLTemplates
}

func (a *Adapter) FTS() storage.FTS {
	return FTS5{}
}

func (a *Adapter) CreateIndex(ctx context.Context, db *sql.DB, schemaJSON []byte) error {
	if _, err := db.ExecContext(ctx, ddlBase); err != nil {
		return err
	}
	_, _ = db.ExecContext(ctx, "PRAGMA journal_mode=WAL;")
	_, _ = db.ExecContext(ctx, "PRAGMA synchronous=NORMAL;")
	_, _ = db.ExecContext(ctx, "PRAGMA foreign_keys=ON;")

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

	// Parse schema to create FTS if needed
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
	_, _ = db.ExecContext(ctx, "INSERT INTO search(search) VALUES('optimize')")
	_, _ = db.ExecContext(ctx, "VACUUM")
	return nil
}

type fieldSpec struct {
	Type   string
	Multi  bool
	Weight *float64
}

// parseSchema parses schema JSON and returns a storage.Schema compatible wrapper
func parseSchema(schemaJSON []byte) (storage.Schema, error) {
	var rawSchema struct {
		Fields map[string]struct {
			Type   string   `json:"type"`
			Multi  bool     `json:"multi,omitempty"`
			Weight *float64 `json:"weight,omitempty"`
		} `json:"fields"`
	}

	if err := json.Unmarshal(schemaJSON, &rawSchema); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	// Convert to our internal type
	fields := make(map[string]fieldSpec)
	for name, spec := range rawSchema.Fields {
		fields[name] = fieldSpec{
			Type:   spec.Type,
			Multi:  spec.Multi,
			Weight: spec.Weight,
		}
	}

	return &parsedSchema{
		data:   schemaJSON,
		fields: fields,
	}, nil
}

// parsedSchema implements storage.Schema interface
type parsedSchema struct {
	data   []byte
	fields map[string]fieldSpec
}

func (s *parsedSchema) ToJSON() ([]byte, error) {
	return s.data, nil
}

func (s *parsedSchema) TextFieldsInOrder() []storage.TextField {
	var result []storage.TextField
	var names []string

	// Collect text field names
	for name, spec := range s.fields {
		if spec.Type == "text" {
			names = append(names, name)
		}
	}

	// Sort for consistency
	sort.Strings(names)

	// Build result with weights
	for _, name := range names {
		spec := s.fields[name]
		weight := 1.0
		if spec.Weight != nil {
			weight = *spec.Weight
		}
		result = append(result, storage.TextField{
			Name:   name,
			Weight: weight,
		})
	}

	return result
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
