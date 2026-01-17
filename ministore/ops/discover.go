package ops

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/sqlbuilder"
)

// ValueCount represents a keyword value with its document frequency
type ValueCount struct {
	Value string
	Count uint64
}

// FieldOverview provides information about a field
type FieldOverview struct {
	Field    string
	Type     string
	Multi    bool
	DocCount uint64
	Unique   *uint64
	Weight   *float64
	Examples []string
}

func ph(style sqlbuilder.PlaceholderStyle, n int) string {
	if style == sqlbuilder.PlaceholderDollar {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// DiscoverValues returns top keyword values for a field
func DiscoverValues(ctx context.Context, db *sql.DB, adapter storage.Adapter, schema storage.Schema, field string, whereSQL string, whereArgs []any, top int) ([]ValueCount, error) {
	// Validate field exists and is keyword
	spec, ok := schema.Get(field)
	if !ok {
		return nil, fmt.Errorf("unknown field: %s", field)
	}
	if spec.Type != storage.FieldType("keyword") {
		return nil, fmt.Errorf("field %s is not a keyword field (type: %s)", field, spec.Type)
	}

	if top <= 0 {
		top = 20
	}

	style := adapter.PlaceholderStyle()

	var querySQL string
	var args []any

	if whereSQL == "" {
		// Simple case: no filter, query from dict directly
		querySQL = fmt.Sprintf(`
			SELECT d.value, d.doc_freq
			FROM kw_dict d
			WHERE d.field = %s
			ORDER BY d.doc_freq DESC, d.value ASC
			LIMIT %s
		`, ph(style, 1), ph(style, 2))
		args = []any{field, top}
	} else {
		// Filtered case: join with postings and filter by result set
		base := len(whereArgs)
		querySQL = fmt.Sprintf(`
			WITH filtered AS (%s)
			SELECT d.value, COUNT(DISTINCT p.item_id) as cnt
			FROM kw_dict d
			JOIN kw_postings p ON p.value_id = d.id
			JOIN filtered f ON f.item_id = p.item_id
			WHERE d.field = %s
			GROUP BY d.value
			ORDER BY cnt DESC, d.value ASC
			LIMIT %s
		`, whereSQL, ph(style, base+1), ph(style, base+2))
		args = append(whereArgs, field, top)
	}

	rows, err := db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query values: %w", err)
	}
	defer rows.Close()

	var result []ValueCount
	for rows.Next() {
		var vc ValueCount
		if err := rows.Scan(&vc.Value, &vc.Count); err != nil {
			return nil, fmt.Errorf("scan value: %w", err)
		}
		result = append(result, vc)
	}

	return result, rows.Err()
}

// DiscoverFields returns an overview of all schema fields
func DiscoverFields(ctx context.Context, db *sql.DB, adapter storage.Adapter, schema storage.Schema) ([]FieldOverview, error) {
	style := adapter.PlaceholderStyle()
	p1 := ph(style, 1)

	var result []FieldOverview

	// Get text fields
	for _, tf := range schema.TextFieldsInOrder() {
		spec, _ := schema.Get(tf.Name)

		// Count documents with this field
		var docCount uint64
		err := db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM field_present WHERE field = %s", p1),
			tf.Name,
		).Scan(&docCount)
		if err != nil {
			return nil, fmt.Errorf("count docs for %s: %w", tf.Name, err)
		}

		weight := tf.Weight
		result = append(result, FieldOverview{
			Field:    tf.Name,
			Type:     string(spec.Type),
			Multi:    spec.Multi,
			DocCount: docCount,
			Weight:   &weight,
			Examples: []string{"(text)"},
		})
	}

	// Get all other fields by querying field_present for unique fields
	// and then looking them up in schema
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT field FROM (
			SELECT field FROM field_present
			UNION SELECT field FROM kw_dict
			UNION SELECT field FROM field_number
			UNION SELECT field FROM field_date
			UNION SELECT field FROM field_bool
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	defer rows.Close()

	seenFields := make(map[string]bool)
	for _, tf := range schema.TextFieldsInOrder() {
		seenFields[tf.Name] = true
	}

	for rows.Next() {
		var fieldName string
		if err := rows.Scan(&fieldName); err != nil {
			return nil, fmt.Errorf("scan field: %w", err)
		}

		if seenFields[fieldName] {
			continue
		}
		seenFields[fieldName] = true

		spec, ok := schema.Get(fieldName)
		if !ok {
			continue
		}

		overview := FieldOverview{
			Field: fieldName,
			Type:  string(spec.Type),
			Multi: spec.Multi,
		}

		// Count documents
		err := db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM field_present WHERE field = %s", p1),
			fieldName,
		).Scan(&overview.DocCount)
		if err != nil {
			return nil, fmt.Errorf("count docs for %s: %w", fieldName, err)
		}

		// Type-specific info
		switch spec.Type {
		case storage.FieldType("keyword"):
			// Count unique values
			var unique uint64
			err := db.QueryRowContext(ctx,
				fmt.Sprintf("SELECT COUNT(*) FROM kw_dict WHERE field = %s", p1),
				fieldName,
			).Scan(&unique)
			if err != nil {
				return nil, fmt.Errorf("count unique for %s: %w", fieldName, err)
			}
			overview.Unique = &unique

			// Get top examples
			exRows, err := db.QueryContext(ctx,
				fmt.Sprintf("SELECT value FROM kw_dict WHERE field = %s ORDER BY doc_freq DESC LIMIT 5", p1),
				fieldName,
			)
			if err != nil {
				return nil, fmt.Errorf("get examples for %s: %w", fieldName, err)
			}
			for exRows.Next() {
				var val string
				if err := exRows.Scan(&val); err != nil {
					exRows.Close()
					return nil, fmt.Errorf("scan example: %w", err)
				}
				overview.Examples = append(overview.Examples, val)
			}
			exRows.Close()

		case storage.FieldType("number"):
			// Get min/max as examples
			var minVal, maxVal sql.NullFloat64
			db.QueryRowContext(ctx,
				fmt.Sprintf("SELECT MIN(value), MAX(value) FROM field_number WHERE field = %s", p1),
				fieldName,
			).Scan(&minVal, &maxVal)
			if minVal.Valid {
				overview.Examples = append(overview.Examples, fmt.Sprintf("min: %g", minVal.Float64))
			}
			if maxVal.Valid {
				overview.Examples = append(overview.Examples, fmt.Sprintf("max: %g", maxVal.Float64))
			}

		case storage.FieldType("date"):
			// Get min/max as examples
			var minVal, maxVal sql.NullInt64
			db.QueryRowContext(ctx,
				fmt.Sprintf("SELECT MIN(value), MAX(value) FROM field_date WHERE field = %s", p1),
				fieldName,
			).Scan(&minVal, &maxVal)
			if minVal.Valid {
				overview.Examples = append(overview.Examples, fmt.Sprintf("min: %d", minVal.Int64))
			}
			if maxVal.Valid {
				overview.Examples = append(overview.Examples, fmt.Sprintf("max: %d", maxVal.Int64))
			}

		case storage.FieldType("bool"):
			// Count true/false
			var trueCount, falseCount int64
			db.QueryRowContext(ctx,
				fmt.Sprintf("SELECT COUNT(*) FROM field_bool WHERE field = %s AND value = 1", p1),
				fieldName,
			).Scan(&trueCount)
			db.QueryRowContext(ctx,
				fmt.Sprintf("SELECT COUNT(*) FROM field_bool WHERE field = %s AND value = 0", p1),
				fieldName,
			).Scan(&falseCount)
			overview.Examples = append(overview.Examples, fmt.Sprintf("true: %d, false: %d", trueCount, falseCount))
		}

		result = append(result, overview)
	}

	return result, rows.Err()
}
