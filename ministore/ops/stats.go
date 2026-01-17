package ops

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

// StatsResult contains statistics for a field
type StatsResult struct {
	Field  string
	Count  uint64
	Min    *float64
	Max    *float64
	Avg    *float64
	Median *float64
}

// Stats computes statistics for a numeric or date field
func Stats(ctx context.Context, db *sql.DB, adapter storage.Adapter, schema storage.Schema, field string, whereSQL string, whereArgs []any) (*StatsResult, error) {
	style := adapter.PlaceholderStyle()

	// Handle implicit created/updated fields
	if field == "created" || field == "updated" {
		col := "created_at"
		if field == "updated" {
			col = "updated_at"
		}
		return statsFromItemsColumn(ctx, db, style, field, col, whereSQL, whereArgs)
	}

	// Validate field exists
	spec, ok := schema.Get(field)
	if !ok {
		return nil, fmt.Errorf("unknown field: %s", field)
	}

	// Must be number or date
	if spec.Type != storage.FieldType("number") && spec.Type != storage.FieldType("date") {
		return nil, fmt.Errorf("stats only available for number/date fields, got %s", spec.Type)
	}

	table := "field_number"
	if spec.Type == storage.FieldType("date") {
		table = "field_date"
	}

	if whereSQL == "" {
		return statsFromTable(ctx, db, style, field, table)
	}
	return statsFromTableFiltered(ctx, db, style, field, table, whereSQL, whereArgs)
}

func statsFromItemsColumn(ctx context.Context, db *sql.DB, style sqlbuilder.PlaceholderStyle, field, col, whereSQL string, whereArgs []any) (*StatsResult, error) {
	result := &StatsResult{Field: field}

	var querySQL string
	var args []any

	if whereSQL == "" {
		querySQL = fmt.Sprintf(`
			SELECT COUNT(*), MIN(%s), MAX(%s), AVG(%s)
			FROM items
		`, col, col, col)
	} else {
		querySQL = fmt.Sprintf(`
			WITH filtered AS (%s)
			SELECT COUNT(*), MIN(i.%s), MAX(i.%s), AVG(i.%s)
			FROM items i
			JOIN filtered f ON f.item_id = i.id
		`, whereSQL, col, col, col)
		args = whereArgs
	}

	var count uint64
	var minVal, maxVal, avgVal sql.NullFloat64
	err := db.QueryRowContext(ctx, querySQL, args...).Scan(&count, &minVal, &maxVal, &avgVal)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}

	result.Count = count
	if minVal.Valid {
		result.Min = &minVal.Float64
	}
	if maxVal.Valid {
		result.Max = &maxVal.Float64
	}
	if avgVal.Valid {
		result.Avg = &avgVal.Float64
	}

	// Calculate median
	if count > 0 {
		median, err := medianFromItemsColumn(ctx, db, style, col, whereSQL, whereArgs, count)
		if err == nil {
			result.Median = median
		}
	}

	return result, nil
}

func statsFromTable(ctx context.Context, db *sql.DB, style sqlbuilder.PlaceholderStyle, field, table string) (*StatsResult, error) {
	result := &StatsResult{Field: field}

	querySQL := fmt.Sprintf(`
		SELECT COUNT(*), MIN(value), MAX(value), AVG(value)
		FROM %s
		WHERE field = %s
	`, table, ph(style, 1))

	var count uint64
	var minVal, maxVal, avgVal sql.NullFloat64
	err := db.QueryRowContext(ctx, querySQL, field).Scan(&count, &minVal, &maxVal, &avgVal)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}

	result.Count = count
	if minVal.Valid {
		result.Min = &minVal.Float64
	}
	if maxVal.Valid {
		result.Max = &maxVal.Float64
	}
	if avgVal.Valid {
		result.Avg = &avgVal.Float64
	}

	// Calculate median
	if count > 0 {
		median, err := medianFromTable(ctx, db, style, table, field, count)
		if err == nil {
			result.Median = median
		}
	}

	return result, nil
}

func statsFromTableFiltered(ctx context.Context, db *sql.DB, style sqlbuilder.PlaceholderStyle, field, table, whereSQL string, whereArgs []any) (*StatsResult, error) {
	result := &StatsResult{Field: field}

	base := len(whereArgs)
	querySQL := fmt.Sprintf(`
		WITH filtered AS (%s)
		SELECT COUNT(*), MIN(t.value), MAX(t.value), AVG(t.value)
		FROM %s t
		JOIN filtered f ON f.item_id = t.item_id
		WHERE t.field = %s
	`, whereSQL, table, ph(style, base+1))

	args := append(whereArgs, field)

	var count uint64
	var minVal, maxVal, avgVal sql.NullFloat64
	err := db.QueryRowContext(ctx, querySQL, args...).Scan(&count, &minVal, &maxVal, &avgVal)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}

	result.Count = count
	if minVal.Valid {
		result.Min = &minVal.Float64
	}
	if maxVal.Valid {
		result.Max = &maxVal.Float64
	}
	if avgVal.Valid {
		result.Avg = &avgVal.Float64
	}

	// Calculate median
	if count > 0 {
		median, err := medianFromTableFiltered(ctx, db, style, table, field, whereSQL, whereArgs, count)
		if err == nil {
			result.Median = median
		}
	}

	return result, nil
}

func medianFromTable(ctx context.Context, db *sql.DB, style sqlbuilder.PlaceholderStyle, table, field string, count uint64) (*float64, error) {
	offset := (count - 1) / 2

	querySQL := fmt.Sprintf(`
		SELECT value FROM %s
		WHERE field = %s
		ORDER BY value
		LIMIT 1 OFFSET %s
	`, table, ph(style, 1), ph(style, 2))

	var val1 float64
	err := db.QueryRowContext(ctx, querySQL, field, offset).Scan(&val1)
	if err != nil {
		return nil, err
	}

	// For even count, average middle two values
	if count%2 == 0 {
		var val2 float64
		err := db.QueryRowContext(ctx, querySQL, field, offset+1).Scan(&val2)
		if err != nil {
			return nil, err
		}
		median := (val1 + val2) / 2
		return &median, nil
	}

	return &val1, nil
}

func medianFromTableFiltered(ctx context.Context, db *sql.DB, style sqlbuilder.PlaceholderStyle, table, field, whereSQL string, whereArgs []any, count uint64) (*float64, error) {
	offset := (count - 1) / 2

	base := len(whereArgs)
	querySQL := fmt.Sprintf(`
		WITH filtered AS (%s)
		SELECT t.value FROM %s t
		JOIN filtered f ON f.item_id = t.item_id
		WHERE t.field = %s
		ORDER BY t.value
		LIMIT 1 OFFSET %s
	`, whereSQL, table, ph(style, base+1), ph(style, base+2))

	args := append(whereArgs, field, offset)

	var val1 float64
	err := db.QueryRowContext(ctx, querySQL, args...).Scan(&val1)
	if err != nil {
		return nil, err
	}

	// For even count, average middle two values
	if count%2 == 0 {
		args2 := append(whereArgs, field, offset+1)
		var val2 float64
		err := db.QueryRowContext(ctx, querySQL, args2...).Scan(&val2)
		if err != nil {
			return nil, err
		}
		median := (val1 + val2) / 2
		return &median, nil
	}

	return &val1, nil
}

func medianFromItemsColumn(ctx context.Context, db *sql.DB, style sqlbuilder.PlaceholderStyle, col, whereSQL string, whereArgs []any, count uint64) (*float64, error) {
	offset := (count - 1) / 2

	var querySQL string
	var args []any

	if whereSQL == "" {
		querySQL = fmt.Sprintf(`
			SELECT %s FROM items
			ORDER BY %s
			LIMIT 1 OFFSET %s
		`, col, col, ph(style, 1))
		args = []any{offset}
	} else {
		base := len(whereArgs)
		querySQL = fmt.Sprintf(`
			WITH filtered AS (%s)
			SELECT i.%s FROM items i
			JOIN filtered f ON f.item_id = i.id
			ORDER BY i.%s
			LIMIT 1 OFFSET %s
		`, whereSQL, col, col, ph(style, base+1))
		args = append(whereArgs, offset)
	}

	var val1 float64
	err := db.QueryRowContext(ctx, querySQL, args...).Scan(&val1)
	if err != nil {
		return nil, err
	}

	// For even count, average middle two values
	if count%2 == 0 {
		var args2 []any
		if whereSQL == "" {
			args2 = []any{offset + 1}
		} else {
			args2 = append(whereArgs, offset+1)
		}

		var val2 float64
		err := db.QueryRowContext(ctx, querySQL, args2...).Scan(&val2)
		if err != nil {
			return nil, err
		}
		median := (val1 + val2) / 2
		return &median, nil
	}

	return &val1, nil
}
