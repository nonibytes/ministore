package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ministore/ministore/ministore/planner"
	"github.com/ministore/ministore/ministore/query"
	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

// SearchOptions configures a search operation
type SearchOptions struct {
	Rank       planner.RankMode
	Limit      int
	After      string // cursor token
	CursorMode CursorMode
	Show       OutputFieldSelector
	Explain    bool
}

// CursorMode specifies cursor type
type CursorMode string

const (
	CursorFull  CursorMode = "full"
	CursorShort CursorMode = "short"
)

// OutputFieldSelector specifies which fields to include in output
type OutputFieldSelector struct {
	Kind   OutputFieldKind
	Fields []string
}

// OutputFieldKind is the type of output selection
type OutputFieldKind int

const (
	ShowNone OutputFieldKind = iota
	ShowAll
	ShowFields
)

// SearchResult is the result of a search operation
type SearchResult struct {
	Items        [][]byte // output-shaped JSON per item
	NextCursor   string
	HasMore      bool
	ExplainSQL   string
	ExplainSteps []string
}

// SearchRow is a raw row from the search query
type SearchRow struct {
	ItemID    int64
	Path      string
	DataJSON  string
	CreatedAt int64
	UpdatedAt int64
	Score     *float64
}

// Search executes a search query
func Search(
	ctx context.Context,
	db *sql.DB,
	adapter storage.Adapter,
	schema storage.Schema,
	queryStr string,
	opts SearchOptions,
	nowMS int64,
	cursorStore CursorStore,
) (*SearchResult, error) {
	// 1. Parse query
	expr, err := query.Parse(queryStr)
	if err != nil {
		return nil, fmt.Errorf("parse query: %w", err)
	}

	// 2. Normalize (validate positive anchor and guardrails)
	normalizedExpr, err := query.Normalize(expr, query.DefaultNormalizeOptions())
	if err != nil {
		return nil, fmt.Errorf("normalize query: %w", err)
	}

	// 3. Create builder for placeholder management
	builder := sqlbuilder.New(adapter.PlaceholderStyle())

	// 4. Compile to CTEs
	compiled, err := planner.Compile(schema, builder, normalizedExpr, nowMS)
	if err != nil {
		return nil, fmt.Errorf("compile query: %w", err)
	}

	// 5. Resolve cursor if present
	var afterFilter string
	if opts.After != "" {
		cursor, err := cursorStore.Resolve(ctx, opts.After)
		if err != nil {
			return nil, fmt.Errorf("resolve cursor: %w", err)
		}

		// Build after filter based on rank mode and cursor payload
		hasFTS := compiled.RequiresFTSJoin
		afterFilter, err = planner.BuildAfterFilter(
			opts.Rank,
			hasFTS,
			builder,
			cursor.Score,
			cursor.ItemID,
			cursor.UpdatedAtMS,
			cursor.Path,
		)
		if err != nil {
			return nil, fmt.Errorf("build after filter: %w", err)
		}
	}

	// 6. Build final SQL
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	limitPlusOne := limit + 1

	searchSQL, err := planner.BuildSearchSQL(schema, compiled, opts.Rank, limitPlusOne, afterFilter, builder)
	if err != nil {
		return nil, fmt.Errorf("build search SQL: %w", err)
	}

	// 7. Execute query
	rows, err := db.QueryContext(ctx, searchSQL, builder.Args()...)
	if err != nil {
		return nil, fmt.Errorf("execute search: %w", err)
	}
	defer rows.Close()

	var searchRows []SearchRow
	for rows.Next() {
		var row SearchRow
		var score sql.NullFloat64
		if err := rows.Scan(&row.ItemID, &row.Path, &row.DataJSON, &row.CreatedAt, &row.UpdatedAt, &score); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if score.Valid {
			row.Score = &score.Float64
		}
		searchRows = append(searchRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// 8. Check for more results
	hasMore := len(searchRows) > limit
	if hasMore {
		searchRows = searchRows[:limit]
	}

	// 9. Shape output
	result := &SearchResult{
		HasMore: hasMore,
	}

	if opts.Explain {
		result.ExplainSQL = searchSQL
		result.ExplainSteps = compiled.ExplainSteps
	}

	for _, row := range searchRows {
		shaped, err := shapeOutput(row, opts.Show)
		if err != nil {
			return nil, fmt.Errorf("shape output: %w", err)
		}
		result.Items = append(result.Items, shaped)
	}

	// 10. Build next cursor from last row
	if hasMore && len(searchRows) > 0 {
		lastRow := searchRows[len(searchRows)-1]
		cursor := CursorPayload{
			ItemID:      lastRow.ItemID,
			Path:        lastRow.Path,
			UpdatedAtMS: lastRow.UpdatedAt,
		}
		if lastRow.Score != nil {
			cursor.Score = *lastRow.Score
		}

		// Determine cursor kind based on rank mode
		switch opts.Rank.Kind {
		case planner.RankDefault:
			if compiled.RequiresFTSJoin {
				cursor.Kind = CursorKindFTS
			} else {
				cursor.Kind = CursorKindRecency
			}
		case planner.RankRecency:
			cursor.Kind = CursorKindRecency
		case planner.RankField:
			cursor.Kind = CursorKindField
			cursor.Field = opts.Rank.Field
			cursor.RankValue = cursor.Score
		case planner.RankNone:
			cursor.Kind = CursorKindNone
		}

		nextCursor, err := cursorStore.Store(ctx, cursor, opts.CursorMode)
		if err != nil {
			return nil, fmt.Errorf("store cursor: %w", err)
		}
		result.NextCursor = nextCursor
	}

	return result, nil
}

// shapeOutput shapes a search row for output based on field selector
func shapeOutput(row SearchRow, show OutputFieldSelector) ([]byte, error) {
	switch show.Kind {
	case ShowNone:
		// Just return path
		output := map[string]interface{}{"path": row.Path}
		return json.Marshal(output)

	case ShowAll:
		// Return entire document (ensure path is present)
		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(row.DataJSON), &doc); err != nil {
			return nil, err
		}
		if _, ok := doc["path"]; !ok {
			doc["path"] = row.Path
		}
		return json.Marshal(doc)

	case ShowFields:
		// Return path + selected fields
		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(row.DataJSON), &doc); err != nil {
			return nil, err
		}

		output := map[string]interface{}{"path": row.Path}
		for _, field := range show.Fields {
			if val, ok := doc[field]; ok {
				output[field] = val
			}
		}
		return json.Marshal(output)

	default:
		return json.Marshal(map[string]interface{}{"path": row.Path})
	}
}

// CursorKind specifies the cursor payload type
type CursorKind string

const (
	CursorKindFTS     CursorKind = "fts"
	CursorKindRecency CursorKind = "recency"
	CursorKindField   CursorKind = "field"
	CursorKindNone    CursorKind = "none"
)

// CursorPayload holds cursor state
type CursorPayload struct {
	Kind        CursorKind `json:"kind"`
	Score       float64    `json:"score,omitempty"`
	ItemID      int64      `json:"item_id,omitempty"`
	UpdatedAtMS int64      `json:"updated_at_ms,omitempty"`
	Path        string     `json:"path,omitempty"`
	Field       string     `json:"field,omitempty"`
	RankValue   float64    `json:"rank_value,omitempty"`
}

// CursorStore abstracts cursor storage
type CursorStore interface {
	Resolve(ctx context.Context, token string) (*CursorPayload, error)
	Store(ctx context.Context, payload CursorPayload, mode CursorMode) (string, error)
}
