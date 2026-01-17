package ministore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ministore/ministore/ministore/ops"
	"github.com/ministore/ministore/ministore/planner"
	"github.com/ministore/ministore/ministore/query"
	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

// Index represents an open ministore index
type Index struct {
	adapter     storage.Adapter
	db          *sql.DB
	schema      Schema
	opts        IndexOptions
	cursorStore ops.CursorStore
}

// Create creates a new index with the given schema
func Create(ctx context.Context, adapter storage.Adapter, schema Schema, opts IndexOptions) (*Index, error) {
	if err := schema.Validate(); err != nil {
		return nil, err
	}

	db, err := adapter.Connect(ctx)
	if err != nil {
		return nil, Wrap(ErrIO, "connect to database", err)
	}

	schemaJSON, err := schema.ToJSON()
	if err != nil {
		return nil, err
	}

	if err := adapter.CreateIndex(ctx, db, schemaJSON); err != nil {
		db.Close()
		return nil, Wrap(ErrSQL, "create index", err)
	}

	return &Index{
		adapter:     adapter,
		db:          db,
		schema:      schema,
		opts:        opts,
		cursorStore: ops.NewDBCursorStore(db, adapter.SQL(), opts.CursorTTL),
	}, nil
}

// Open opens an existing index
func Open(ctx context.Context, adapter storage.Adapter, opts IndexOptions) (*Index, error) {
	db, err := adapter.Connect(ctx)
	if err != nil {
		return nil, Wrap(ErrIO, "connect to database", err)
	}

	schemaJSON, err := adapter.OpenIndex(ctx, db)
	if err != nil {
		db.Close()
		return nil, Wrap(ErrSQL, "open index", err)
	}

	schema, err := SchemaFromJSON(schemaJSON)
	if err != nil {
		db.Close()
		return nil, err
	}

	// Verify FTS structure matches schema
	if err := adapter.VerifyFTS(ctx, db, schema.AsStorageSchema()); err != nil {
		db.Close()
		return nil, Wrap(ErrSchema, "FTS verification failed", err)
	}

	return &Index{
		adapter:     adapter,
		db:          db,
		schema:      schema,
		opts:        opts,
		cursorStore: ops.NewDBCursorStore(db, adapter.SQL(), opts.CursorTTL),
	}, nil
}

// Close closes the index
func (ix *Index) Close() error {
	if ix.db != nil {
		if err := ix.db.Close(); err != nil {
			return Wrap(ErrIO, "close database", err)
		}
	}
	return ix.adapter.Close()
}

// Schema returns the index schema
func (ix *Index) Schema() Schema {
	return ix.schema
}

// PutJSON inserts or updates an item from JSON
func (ix *Index) PutJSON(ctx context.Context, docJSON []byte) error {
	// Prepare the put operation
	prep, err := ops.PreparePut(ix.schema.AsStorageSchema(), docJSON)
	if err != nil {
		return Wrap(ErrSchema, "prepare put", err)
	}

	// Execute in transaction
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return Wrap(ErrSQL, "begin transaction", err)
	}
	defer tx.Rollback()

	sqlt := ix.adapter.SQL()
	fts := ix.adapter.FTS()
	nowMS := ix.nowMS()

	_, _, err = ops.ExecutePut(ctx, tx, sqlt, fts, ix.schema.AsStorageSchema(), prep, nowMS)
	if err != nil {
		return Wrap(ErrSQL, "execute put", err)
	}

	if err := tx.Commit(); err != nil {
		return Wrap(ErrSQL, "commit", err)
	}

	return nil
}

// PutFields inserts or updates an item with field values
func (ix *Index) PutFields(ctx context.Context, path string, fieldsJSON []byte) error {
	// Build full document JSON with path
	// This is a convenience method that wraps fields in a document
	doc := make(map[string]interface{})
	if err := unmarshalJSON(fieldsJSON, &doc); err != nil {
		return Wrap(ErrSchema, "invalid fields JSON", err)
	}
	doc["path"] = path

	docJSON, err := marshalJSON(doc)
	if err != nil {
		return Wrap(ErrSchema, "marshal document", err)
	}

	return ix.PutJSON(ctx, docJSON)
}

// Get retrieves an item by path
func (ix *Index) Get(ctx context.Context, path string) (ItemView, error) {
	sqlt := ix.adapter.SQL()
	var itemID int64
	var dataJSON string
	var createdAt, updatedAt int64

	err := ix.db.QueryRowContext(ctx, sqlt.GetItemByPath, path).Scan(&itemID, &dataJSON, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return ItemView{}, NotFoundError(path)
	}
	if err != nil {
		return ItemView{}, Wrap(ErrSQL, "get item", err)
	}

	return ItemView{
		Path:    path,
		DocJSON: []byte(dataJSON),
		Meta: ItemMeta{
			CreatedAtMS: createdAt,
			UpdatedAtMS: updatedAt,
		},
	}, nil
}

// Peek retrieves just the raw JSON for an item
func (ix *Index) Peek(ctx context.Context, path string) ([]byte, error) {
	view, err := ix.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	return view.DocJSON, nil
}

// Delete removes an item by path
func (ix *Index) Delete(ctx context.Context, path string) (bool, error) {
	sqlt := ix.adapter.SQL()
	fts := ix.adapter.FTS()

	return ops.DeleteByPath(ctx, ix.db, sqlt, fts, path)
}

// DeleteWhere deletes items matching a query
func (ix *Index) DeleteWhere(ctx context.Context, queryStr string) (int, error) {
	// Parse and compile query
	expr, err := query.Parse(queryStr)
	if err != nil {
		return 0, Wrap(ErrQueryParse, "parse query", err)
	}

	normalizedExpr, err := query.Normalize(expr, query.DefaultNormalizeOptions())
	if err != nil {
		return 0, Wrap(ErrQueryRejected, "normalize query", err)
	}

	builder := sqlbuilder.New(ix.adapter.PlaceholderStyle())
	compiled, err := planner.Compile(ix.adapter, ix.schema.AsStorageSchema(), builder, normalizedExpr, ix.nowMS())
	if err != nil {
		return 0, Wrap(ErrQueryRejected, "compile query", err)
	}

	// Build CTE parts
	var cteParts []string
	for _, cte := range compiled.CTEs {
		cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", cte.Name, cte.SQL))
	}

	sqlt := ix.adapter.SQL()
	fts := ix.adapter.FTS()

	return ops.DeleteWhere(ctx, ix.db, sqlt, fts, compiled.ResultCTE, cteParts, builder.Args())
}

// Search executes a query and returns results
func (ix *Index) Search(ctx context.Context, queryStr string, sopts SearchOptions) (SearchResultPage, error) {
	// Clean up expired cursors (best effort)
	if dbcs, ok := ix.cursorStore.(*ops.DBCursorStore); ok {
		_ = dbcs.CleanupExpired(ctx)
	}

	// Convert ministore.SearchOptions to ops.SearchOptions
	opsOpts := ops.SearchOptions{
		Rank: planner.RankMode{
			Kind:  toRankKind(sopts.Rank.Kind),
			Field: sopts.Rank.Field,
		},
		Limit:      sopts.Limit,
		After:      sopts.After,
		CursorMode: ops.CursorMode(sopts.CursorMode),
		Show: ops.OutputFieldSelector{
			Kind:   toOutputFieldKind(sopts.Show.Kind),
			Fields: sopts.Show.Fields,
		},
		Explain: sopts.Explain,
	}

	result, err := ops.Search(
		ctx,
		ix.db,
		ix.adapter,
		ix.schema.AsStorageSchema(),
		queryStr,
		opsOpts,
		ix.nowMS(),
		ix.cursorStore,
	)
	if err != nil {
		return SearchResultPage{}, Wrap(ErrSQL, "search", err)
	}

	return SearchResultPage{
		Items:        result.Items,
		NextCursor:   result.NextCursor,
		HasMore:      result.HasMore,
		ExplainSQL:   result.ExplainSQL,
		ExplainSteps: result.ExplainSteps,
	}, nil
}

// DiscoverValues lists unique values for a field
func (ix *Index) DiscoverValues(ctx context.Context, field string, where string, top int) ([]ValueCount, error) {
	var whereSQL string
	var whereArgs []any

	if where != "" {
		// Compile the where query to a CTE
		expr, err := query.Parse(where)
		if err != nil {
			return nil, Wrap(ErrQueryParse, "parse where", err)
		}

		normalizedExpr, err := query.Normalize(expr, query.DefaultNormalizeOptions())
		if err != nil {
			return nil, Wrap(ErrQueryRejected, "normalize where", err)
		}

		builder := sqlbuilder.New(ix.adapter.PlaceholderStyle())
		compiled, err := planner.Compile(ix.adapter, ix.schema.AsStorageSchema(), builder, normalizedExpr, ix.nowMS())
		if err != nil {
			return nil, Wrap(ErrQueryRejected, "compile where", err)
		}

		// Build CTEs
		var cteParts []string
		for _, cte := range compiled.CTEs {
			cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", cte.Name, cte.SQL))
		}

		if len(cteParts) > 0 {
			whereSQL = "WITH " + joinComma(cteParts) + " SELECT item_id FROM " + compiled.ResultCTE
		} else {
			whereSQL = "SELECT item_id FROM " + compiled.ResultCTE
		}
		whereArgs = builder.Args()
	}

	results, err := ops.DiscoverValues(ctx, ix.db, ix.adapter, ix.schema.AsStorageSchema(), field, whereSQL, whereArgs, top)
	if err != nil {
		return nil, Wrap(ErrSQL, "discover values", err)
	}

	// Convert ops.ValueCount to ministore.ValueCount
	var converted []ValueCount
	for _, r := range results {
		converted = append(converted, ValueCount{Value: r.Value, Count: r.Count})
	}
	return converted, nil
}

// DiscoverFields returns an overview of all fields
func (ix *Index) DiscoverFields(ctx context.Context) ([]FieldOverview, error) {
	results, err := ops.DiscoverFields(ctx, ix.db, ix.adapter, ix.schema.AsStorageSchema())
	if err != nil {
		return nil, Wrap(ErrSQL, "discover fields", err)
	}

	// Convert ops.FieldOverview to ministore.FieldOverview
	var converted []FieldOverview
	for _, r := range results {
		converted = append(converted, FieldOverview{
			Field:    r.Field,
			Type:     FieldType(r.Type),
			Multi:    r.Multi,
			DocCount: r.DocCount,
			Unique:   r.Unique,
			Weight:   r.Weight,
			Examples: r.Examples,
		})
	}
	return converted, nil
}

// Stats computes statistics for a field
func (ix *Index) Stats(ctx context.Context, field string, where string) (StatsResult, error) {
	var whereSQL string
	var whereArgs []any

	if where != "" {
		// Compile the where query to a CTE
		expr, err := query.Parse(where)
		if err != nil {
			return StatsResult{}, Wrap(ErrQueryParse, "parse where", err)
		}

		normalizedExpr, err := query.Normalize(expr, query.DefaultNormalizeOptions())
		if err != nil {
			return StatsResult{}, Wrap(ErrQueryRejected, "normalize where", err)
		}

		builder := sqlbuilder.New(ix.adapter.PlaceholderStyle())
		compiled, err := planner.Compile(ix.adapter, ix.schema.AsStorageSchema(), builder, normalizedExpr, ix.nowMS())
		if err != nil {
			return StatsResult{}, Wrap(ErrQueryRejected, "compile where", err)
		}

		// Build CTEs
		var cteParts []string
		for _, cte := range compiled.CTEs {
			cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", cte.Name, cte.SQL))
		}

		if len(cteParts) > 0 {
			whereSQL = "WITH " + joinComma(cteParts) + " SELECT item_id FROM " + compiled.ResultCTE
		} else {
			whereSQL = "SELECT item_id FROM " + compiled.ResultCTE
		}
		whereArgs = builder.Args()
	}

	result, err := ops.Stats(ctx, ix.db, ix.adapter, ix.schema.AsStorageSchema(), field, whereSQL, whereArgs)
	if err != nil {
		return StatsResult{}, Wrap(ErrSQL, "stats", err)
	}

	return StatsResult{
		Field:  result.Field,
		Count:  result.Count,
		Min:    result.Min,
		Max:    result.Max,
		Avg:    result.Avg,
		Median: result.Median,
	}, nil
}

// Optimize optimizes the index (vacuum, FTS optimize, etc.)
func (ix *Index) Optimize(ctx context.Context) error {
	return ix.adapter.Optimize(ctx, ix.db)
}

// ApplySchema applies schema changes (additive only)
func (ix *Index) ApplySchema(ctx context.Context, newSchema Schema) error {
	if err := newSchema.Validate(); err != nil {
		return err
	}
	if err := ix.adapter.ApplySchemaAdditive(ctx, ix.db, ix.schema.AsStorageSchema(), newSchema.AsStorageSchema()); err != nil {
		return Wrap(ErrSQL, "apply schema", err)
	}
	ix.schema = newSchema
	return nil
}

// MigrateRebuild performs a full rebuild with a new schema
func (ix *Index) MigrateRebuild(ctx context.Context, dst storage.Adapter, newSchema Schema) error {
	// TODO: Implement migration logic
	return New(ErrFeature, "MigrateRebuild not yet implemented")
}

// Batch executes a batch of operations
func (ix *Index) Batch(ctx context.Context, b Batch) (int, error) {
	if b.Empty() {
		return 0, nil
	}

	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, Wrap(ErrSQL, "begin transaction", err)
	}
	defer tx.Rollback()

	sqlt := ix.adapter.SQL()
	fts := ix.adapter.FTS()
	nowMS := ix.nowMS()

	count := 0
	for _, op := range b.ops {
		switch op.Kind {
		case batchPut:
			prep, err := ops.PreparePut(ix.schema.AsStorageSchema(), op.Doc)
			if err != nil {
				return count, Wrap(ErrSchema, "prepare put", err)
			}
			_, _, err = ops.ExecutePut(ctx, tx, sqlt, fts, ix.schema.AsStorageSchema(), prep, nowMS)
			if err != nil {
				return count, Wrap(ErrSQL, "execute put", err)
			}
		case batchDelete:
			// Find item ID
			var itemID int64
			var createdAt int64
			err := tx.QueryRowContext(ctx, sqlt.FindItemIDByPath, op.Path).Scan(&itemID, &createdAt)
			if err == sql.ErrNoRows {
				// Item doesn't exist, skip
				continue
			}
			if err != nil {
				return count, Wrap(ErrSQL, "find item", err)
			}
			if err := ops.DeleteByItemID(ctx, tx, sqlt, fts, itemID); err != nil {
				return count, Wrap(ErrSQL, "delete item", err)
			}
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return count, Wrap(ErrSQL, "commit transaction", err)
	}
	return count, nil
}

// Adapter returns the underlying storage adapter
func (ix *Index) Adapter() storage.Adapter {
	return ix.adapter
}

// DB returns the underlying database connection (for advanced use)
func (ix *Index) DB() *sql.DB {
	return ix.db
}

// nowMS returns current time in milliseconds since epoch
func (ix *Index) nowMS() int64 {
	return ix.opts.Now().UnixMilli()
}

// Helper functions

func toRankKind(k RankModeKind) planner.RankKind {
	switch k {
	case RankDefault:
		return planner.RankDefault
	case RankRecency:
		return planner.RankRecency
	case RankField:
		return planner.RankField
	case RankNone:
		return planner.RankNone
	default:
		return planner.RankDefault
	}
}

func toOutputFieldKind(k OutputFieldSelectorKind) ops.OutputFieldKind {
	switch k {
	case ShowNone:
		return ops.ShowNone
	case ShowAll:
		return ops.ShowAll
	case ShowFields:
		return ops.ShowFields
	default:
		return ops.ShowNone
	}
}

func joinComma(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

// Ensure schemaStorageAdapter implements storage.Schema interface
var _ storage.Schema = schemaStorageAdapter{}
