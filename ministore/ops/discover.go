package ops

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ministore/ministore/ministore/storage"
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

// DiscoverValues returns top keyword values for a field
func DiscoverValues(ctx context.Context, db *sql.DB, schema storage.Schema, field string, whereSQL string, whereArgs []any, top int) ([]ValueCount, error) {
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

	var querySQL string
	var args []any

	if whereSQL == "" {
		// Simple case: no filter, query from dict directly
		querySQL = `
			SELECT d.value, d.doc_freq
			FROM kw_dict d
			WHERE d.field = ?
			ORDER BY d.doc_freq DESC, d.value ASC
			LIMIT ?
		`
		args = []any{field, top}
	} else {
		// Filtered case: join with postings and filter by result set
		querySQL = fmt.Sprintf(`
			WITH filtered AS (%s)
			SELECT d.value, COUNT(DISTINCT p.item_id) as cnt
			FROM kw_dict d
			JOIN kw_postings p ON p.value_id = d.id
			JOIN filtered f ON f.item_id = p.item_id
			WHERE d.field = ?
			GROUP BY d.value
			ORDER BY cnt DESC, d.value ASC
			LIMIT ?
		`, whereSQL)
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
func DiscoverFields(ctx context.Context, db *sql.DB, schema storage.Schema) ([]FieldOverview, error) {
	var result []FieldOverview

	// Get text fields
	for _, tf := range schema.TextFieldsInOrder() {
		spec, _ := schema.Get(tf.Name)

		// Count documents with this field
		var docCount uint64
		err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM field_present WHERE field = ?",
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
			"SELECT COUNT(*) FROM field_present WHERE field = ?",
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
				"SELECT COUNT(*) FROM kw_dict WHERE field = ?",
				fieldName,
			).Scan(&unique)
			if err != nil {
				return nil, fmt.Errorf("count unique for %s: %w", fieldName, err)
			}
			overview.Unique = &unique

			// Get top examples
			exRows, err := db.QueryContext(ctx,
				"SELECT value FROM kw_dict WHERE field = ? ORDER BY doc_freq DESC LIMIT 5",
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
				"SELECT MIN(value), MAX(value) FROM field_number WHERE field = ?",
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
				"SELECT MIN(value), MAX(value) FROM field_date WHERE field = ?",
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
				"SELECT COUNT(*) FROM field_bool WHERE field = ? AND value = 1",
				fieldName,
			).Scan(&trueCount)
			db.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM field_bool WHERE field = ? AND value = 0",
				fieldName,
			).Scan(&falseCount)
			overview.Examples = append(overview.Examples, fmt.Sprintf("true: %d, false: %d", trueCount, falseCount))
		}

		result = append(result, overview)
	}

	return result, rows.Err()
}
