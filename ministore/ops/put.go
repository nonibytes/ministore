package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ministore/ministore/ministore/storage"
)

// PutPrepared holds the prepared data for a put operation
type PutPrepared struct {
	Path          string
	DataJSON      []byte
	TextCols      map[string]*string   // nil means absent
	KeywordFields map[string][]string  // field -> values
	NumberFields  map[string][]float64 // field -> values
	DateFieldsMS  map[string][]int64   // field -> epoch ms values
	BoolFields    map[string]bool      // field -> value
	PresentFields []string             // fields that are present
}

// PreparePut validates and extracts fields from a document for indexing
func PreparePut(schema storage.Schema, docJSON []byte) (*PutPrepared, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON document: %w", err)
	}

	// Extract path
	pathVal, ok := doc["path"]
	if !ok {
		return nil, fmt.Errorf("document must contain 'path' field")
	}
	path, ok := pathVal.(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("'path' must be a non-empty string")
	}

	prep := &PutPrepared{
		Path:          path,
		DataJSON:      docJSON,
		TextCols:      make(map[string]*string),
		KeywordFields: make(map[string][]string),
		NumberFields:  make(map[string][]float64),
		DateFieldsMS:  make(map[string][]int64),
		BoolFields:    make(map[string]bool),
	}

	// Process each field in the schema
	for _, tf := range schema.TextFieldsInOrder() {
		fieldName := tf.Name
		val, exists := doc[fieldName]
		if !exists || val == nil {
			prep.TextCols[fieldName] = nil
			continue
		}
		str, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("field '%s' must be a string for text type", fieldName)
		}
		prep.TextCols[fieldName] = &str
		prep.PresentFields = append(prep.PresentFields, fieldName)
	}

	// Process other fields by iterating doc and checking schema
	for fieldName, fieldVal := range doc {
		if fieldName == "path" {
			continue
		}
		if fieldVal == nil {
			continue
		}

		spec, ok := schema.Get(fieldName)
		if !ok {
			// Unknown field - skip (not in schema)
			continue
		}

		switch spec.Type {
		case storage.FieldType("text"):
			// Already handled above
			continue

		case storage.FieldType("keyword"):
			values, err := extractKeywordValues(fieldVal, spec.Multi)
			if err != nil {
				return nil, fmt.Errorf("field '%s': %w", fieldName, err)
			}
			if len(values) > 0 {
				prep.KeywordFields[fieldName] = values
				prep.PresentFields = append(prep.PresentFields, fieldName)
			}

		case storage.FieldType("number"):
			values, err := extractNumberValues(fieldVal, spec.Multi)
			if err != nil {
				return nil, fmt.Errorf("field '%s': %w", fieldName, err)
			}
			if len(values) > 0 {
				prep.NumberFields[fieldName] = values
				prep.PresentFields = append(prep.PresentFields, fieldName)
			}

		case storage.FieldType("date"):
			values, err := extractDateValues(fieldVal, spec.Multi)
			if err != nil {
				return nil, fmt.Errorf("field '%s': %w", fieldName, err)
			}
			if len(values) > 0 {
				prep.DateFieldsMS[fieldName] = values
				prep.PresentFields = append(prep.PresentFields, fieldName)
			}

		case storage.FieldType("bool"):
			val, err := extractBoolValue(fieldVal)
			if err != nil {
				return nil, fmt.Errorf("field '%s': %w", fieldName, err)
			}
			prep.BoolFields[fieldName] = val
			prep.PresentFields = append(prep.PresentFields, fieldName)
		}
	}

	return prep, nil
}

// ExecutePut executes a prepared put operation within a transaction
func ExecutePut(ctx context.Context, tx *sql.Tx, sqlt storage.SQL, fts storage.FTS, schema storage.Schema, prep *PutPrepared, nowMS int64) (itemID int64, createdAtMS int64, err error) {
	// 1. Upsert items row
	itemID, createdAtMS, err = upsertItem(ctx, tx, sqlt, prep.Path, prep.DataJSON, nowMS)
	if err != nil {
		return 0, 0, fmt.Errorf("upsert item: %w", err)
	}

	// 2. Load old keyword value_ids for doc_freq maintenance
	oldValueIDs, err := loadOldValueIDs(ctx, tx, sqlt, itemID)
	if err != nil {
		return 0, 0, fmt.Errorf("load old value_ids: %w", err)
	}

	// 3. Delete old index rows
	if err := deleteOldIndexRows(ctx, tx, sqlt, fts, itemID); err != nil {
		return 0, 0, fmt.Errorf("delete old index rows: %w", err)
	}

	// 4. Insert field_present rows
	for _, field := range prep.PresentFields {
		if _, err := tx.ExecContext(ctx, sqlt.InsertFieldPresent, itemID, field); err != nil {
			return 0, 0, fmt.Errorf("insert field_present: %w", err)
		}
	}

	// 5. Insert keywords with doc_freq maintenance
	newValueIDs := make(map[int64]bool)
	for field, values := range prep.KeywordFields {
		for _, value := range values {
			valueID, err := insertKeyword(ctx, tx, sqlt, field, value)
			if err != nil {
				return 0, 0, fmt.Errorf("insert keyword: %w", err)
			}
			newValueIDs[valueID] = true

			// Insert posting
			if _, err := tx.ExecContext(ctx, sqlt.InsertOrIgnoreKwPosting, field, valueID, itemID); err != nil {
				return 0, 0, fmt.Errorf("insert posting: %w", err)
			}

			// Increment doc_freq only if this value_id was not previously associated
			if !oldValueIDs[valueID] {
				if _, err := tx.ExecContext(ctx, sqlt.IncrementDocFreq, valueID); err != nil {
					return 0, 0, fmt.Errorf("increment doc_freq: %w", err)
				}
			}
		}
	}

	// 6. Decrement doc_freq for removed value_ids
	for valueID := range oldValueIDs {
		if !newValueIDs[valueID] {
			if _, err := tx.ExecContext(ctx, sqlt.DecrementDocFreq, valueID); err != nil {
				return 0, 0, fmt.Errorf("decrement doc_freq: %w", err)
			}
		}
	}

	// 7. Insert numbers
	for field, values := range prep.NumberFields {
		for _, val := range values {
			if _, err := tx.ExecContext(ctx, sqlt.InsertFieldNumber, itemID, field, val); err != nil {
				return 0, 0, fmt.Errorf("insert number: %w", err)
			}
		}
	}

	// 8. Insert dates
	for field, values := range prep.DateFieldsMS {
		for _, val := range values {
			if _, err := tx.ExecContext(ctx, sqlt.InsertFieldDate, itemID, field, val); err != nil {
				return 0, 0, fmt.Errorf("insert date: %w", err)
			}
		}
	}

	// 9. Insert bools
	for field, val := range prep.BoolFields {
		intVal := 0
		if val {
			intVal = 1
		}
		if _, err := tx.ExecContext(ctx, sqlt.InsertFieldBool, itemID, field, intVal); err != nil {
			return 0, 0, fmt.Errorf("insert bool: %w", err)
		}
	}

	// 10. Upsert FTS row
	if fts.HasFTS(schema) {
		if err := fts.UpsertRow(ctx, tx, itemID, schema, prep.TextCols); err != nil {
			return 0, 0, fmt.Errorf("upsert FTS: %w", err)
		}
	}

	return itemID, createdAtMS, nil
}

func upsertItem(ctx context.Context, tx *sql.Tx, sqlt storage.SQL, path string, dataJSON []byte, nowMS int64) (itemID int64, createdAtMS int64, error error) {
	// Check if item exists
	var existingID int64
	var existingCreatedAt int64
	err := tx.QueryRowContext(ctx, sqlt.FindItemIDByPath, path).Scan(&existingID, &existingCreatedAt)

	if err == sql.ErrNoRows {
		// Insert new item
		sql, args := sqlt.UpsertItem.Build(path, dataJSON, nowMS, nowMS, false)
		result, err := tx.ExecContext(ctx, sql, args...)
		if err != nil {
			return 0, 0, err
		}
		itemID, err = result.LastInsertId()
		if err != nil {
			return 0, 0, err
		}
		return itemID, nowMS, nil
	}
	if err != nil {
		return 0, 0, err
	}

	// Update existing item
	sql, args := sqlt.UpsertItemWithTS.Build(path, dataJSON, existingCreatedAt, nowMS, false)
	_, err = tx.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, 0, err
	}
	return existingID, existingCreatedAt, nil
}

func loadOldValueIDs(ctx context.Context, tx *sql.Tx, sqlt storage.SQL, itemID int64) (map[int64]bool, error) {
	result := make(map[int64]bool)
	rows, err := tx.QueryContext(ctx, sqlt.GetValueIDsByItem, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var valueID int64
		if err := rows.Scan(&valueID); err != nil {
			return nil, err
		}
		result[valueID] = true
	}
	return result, rows.Err()
}

func deleteOldIndexRows(ctx context.Context, tx *sql.Tx, sqlt storage.SQL, fts storage.FTS, itemID int64) error {
	// Delete in specific order to avoid FK issues
	queries := []string{
		sqlt.DeletePostingsByItem,
		sqlt.DeleteNumberByItem,
		sqlt.DeleteDateByItem,
		sqlt.DeleteBoolByItem,
		sqlt.DeletePresentByItem,
	}

	for _, q := range queries {
		if _, err := tx.ExecContext(ctx, q, itemID); err != nil {
			return err
		}
	}

	// Delete FTS row (handled specially by FTS driver)
	if err := fts.DeleteRow(ctx, tx, itemID); err != nil {
		return err
	}

	return nil
}

func insertKeyword(ctx context.Context, tx *sql.Tx, sqlt storage.SQL, field, value string) (int64, error) {
	// Insert or ignore into dict
	if _, err := tx.ExecContext(ctx, sqlt.InsertOrIgnoreKwDict, field, value); err != nil {
		return 0, err
	}

	// Get dict ID
	var valueID int64
	err := tx.QueryRowContext(ctx, sqlt.GetKwDictID, field, value).Scan(&valueID)
	if err != nil {
		return 0, err
	}
	return valueID, nil
}

// extractKeywordValues extracts keyword values from a JSON value
func extractKeywordValues(val interface{}, multi bool) ([]string, error) {
	switch v := val.(type) {
	case string:
		return []string{v}, nil
	case float64:
		return []string{strconv.FormatFloat(v, 'f', -1, 64)}, nil
	case bool:
		return []string{strconv.FormatBool(v)}, nil
	case []interface{}:
		if !multi && len(v) > 1 {
			return nil, fmt.Errorf("array not allowed for non-multi field")
		}
		var result []string
		for _, item := range v {
			switch i := item.(type) {
			case string:
				result = append(result, i)
			case float64:
				result = append(result, strconv.FormatFloat(i, 'f', -1, 64))
			case bool:
				result = append(result, strconv.FormatBool(i))
			default:
				return nil, fmt.Errorf("invalid keyword value type: %T", item)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("invalid keyword value type: %T", val)
	}
}

// extractNumberValues extracts number values from a JSON value
func extractNumberValues(val interface{}, multi bool) ([]float64, error) {
	switch v := val.(type) {
	case float64:
		return []float64{v}, nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse '%s' as number", v)
		}
		return []float64{f}, nil
	case []interface{}:
		if !multi && len(v) > 1 {
			return nil, fmt.Errorf("array not allowed for non-multi field")
		}
		var result []float64
		for _, item := range v {
			switch i := item.(type) {
			case float64:
				result = append(result, i)
			case string:
				f, err := strconv.ParseFloat(i, 64)
				if err != nil {
					return nil, fmt.Errorf("cannot parse '%s' as number", i)
				}
				result = append(result, f)
			default:
				return nil, fmt.Errorf("invalid number value type: %T", item)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("invalid number value type: %T", val)
	}
}

// extractDateValues extracts date values as epoch milliseconds
func extractDateValues(val interface{}, multi bool) ([]int64, error) {
	parseDate := func(s string) (int64, error) {
		// Try YYYY-MM-DD
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.UnixMilli(), nil
		}
		// Try RFC3339
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UnixMilli(), nil
		}
		return 0, fmt.Errorf("invalid date format: %s", s)
	}

	switch v := val.(type) {
	case string:
		ms, err := parseDate(v)
		if err != nil {
			return nil, err
		}
		return []int64{ms}, nil
	case []interface{}:
		if !multi && len(v) > 1 {
			return nil, fmt.Errorf("array not allowed for non-multi field")
		}
		var result []int64
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("date value must be string")
			}
			ms, err := parseDate(s)
			if err != nil {
				return nil, err
			}
			result = append(result, ms)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("invalid date value type: %T", val)
	}
}

// extractBoolValue extracts a boolean value
func extractBoolValue(val interface{}) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		switch v {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("invalid bool string: %s", v)
		}
	default:
		return false, fmt.Errorf("invalid bool value type: %T", val)
	}
}

// NowMS returns current time in milliseconds since epoch
func NowMS() int64 {
	return time.Now().UnixMilli()
}
