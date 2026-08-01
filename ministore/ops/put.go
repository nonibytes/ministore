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
	return PreparePutDocument(schema, doc, docJSON)
}

// PreparePutDocument validates and extracts fields from an already decoded
// document. dataJSON is the representation persisted in the items table.
func PreparePutDocument(schema storage.Schema, doc map[string]any, dataJSON []byte) (*PutPrepared, error) {
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
		DataJSON:      dataJSON,
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
func ExecutePut(ctx context.Context, exec storage.SQLExecutor, sqlt storage.SQL, fts storage.FTS, schema storage.Schema, prep *PutPrepared, nowMS int64) (itemID int64, createdAtMS int64, err error) {
	writer := NewPutExecutor(exec, sqlt, fts, schema)
	itemID, createdAtMS, err = writer.Execute(ctx, prep, nowMS)
	if err != nil {
		return 0, 0, err
	}
	if err := writer.FlushDocFreq(ctx); err != nil {
		return 0, 0, err
	}
	return itemID, createdAtMS, nil
}

type keywordCacheKey struct {
	field string
	value string
}

// PutExecutor reuses dictionary lookups and batches document-frequency updates.
// Call FlushWorkingSet periodically when the executor processes an unbounded
// stream so its caches remain bounded by the work since the last flush.
type PutExecutor struct {
	exec         storage.SQLExecutor
	sqlt         storage.SQL
	fts          storage.FTS
	schema       storage.Schema
	keywordIDs   map[keywordCacheKey]int64
	docFreqDelta map[int64]int64
}

func NewPutExecutor(exec storage.SQLExecutor, sqlt storage.SQL, fts storage.FTS, schema storage.Schema) *PutExecutor {
	return &PutExecutor{
		exec:         exec,
		sqlt:         sqlt,
		fts:          fts,
		schema:       schema,
		keywordIDs:   make(map[keywordCacheKey]int64),
		docFreqDelta: make(map[int64]int64),
	}
}

func (w *PutExecutor) Execute(ctx context.Context, prep *PutPrepared, nowMS int64) (itemID int64, createdAtMS int64, err error) {
	itemID, createdAtMS, isNew, err := insertOrUpdateItem(ctx, w.exec, w.sqlt, prep.Path, prep.DataJSON, nowMS)
	if err != nil {
		return 0, 0, fmt.Errorf("upsert item: %w", err)
	}

	oldValueIDs := make(map[int64]bool)
	if !isNew {
		oldValueIDs, err = loadOldValueIDs(ctx, w.exec, w.sqlt, itemID)
		if err != nil {
			return 0, 0, fmt.Errorf("load old value_ids: %w", err)
		}

		if err := deleteOldIndexRows(ctx, w.exec, w.sqlt, w.fts, itemID); err != nil {
			return 0, 0, fmt.Errorf("delete old index rows: %w", err)
		}
	}

	for _, field := range prep.PresentFields {
		if _, err := w.exec.ExecContext(ctx, w.sqlt.InsertFieldPresent, itemID, field); err != nil {
			return 0, 0, fmt.Errorf("insert field_present: %w", err)
		}
	}

	newValueIDs := make(map[int64]bool)
	for field, values := range prep.KeywordFields {
		for _, value := range values {
			valueID, err := w.keywordID(ctx, field, value)
			if err != nil {
				return 0, 0, fmt.Errorf("insert keyword: %w", err)
			}
			if newValueIDs[valueID] {
				continue
			}
			newValueIDs[valueID] = true

			if _, err := w.exec.ExecContext(ctx, w.sqlt.InsertOrIgnoreKwPosting, field, valueID, itemID); err != nil {
				return 0, 0, fmt.Errorf("insert posting: %w", err)
			}

			if !oldValueIDs[valueID] {
				w.docFreqDelta[valueID]++
			}
		}
	}

	for valueID := range oldValueIDs {
		if !newValueIDs[valueID] {
			w.docFreqDelta[valueID]--
		}
	}

	for field, values := range prep.NumberFields {
		for _, val := range values {
			if _, err := w.exec.ExecContext(ctx, w.sqlt.InsertFieldNumber, itemID, field, val); err != nil {
				return 0, 0, fmt.Errorf("insert number: %w", err)
			}
		}
	}

	for field, values := range prep.DateFieldsMS {
		for _, val := range values {
			if _, err := w.exec.ExecContext(ctx, w.sqlt.InsertFieldDate, itemID, field, val); err != nil {
				return 0, 0, fmt.Errorf("insert date: %w", err)
			}
		}
	}

	for field, val := range prep.BoolFields {
		intVal := 0
		if val {
			intVal = 1
		}
		if _, err := w.exec.ExecContext(ctx, w.sqlt.InsertFieldBool, itemID, field, intVal); err != nil {
			return 0, 0, fmt.Errorf("insert bool: %w", err)
		}
	}

	if w.fts.HasFTS(w.schema) {
		if err := w.fts.UpsertRow(ctx, w.exec, itemID, w.schema, prep.TextCols); err != nil {
			return 0, 0, fmt.Errorf("upsert FTS: %w", err)
		}
	}

	return itemID, createdAtMS, nil
}

func (w *PutExecutor) FlushDocFreq(ctx context.Context) error {
	for valueID, delta := range w.docFreqDelta {
		if delta == 0 {
			delete(w.docFreqDelta, valueID)
			continue
		}
		if _, err := w.exec.ExecContext(ctx, w.sqlt.AdjustDocFreq, valueID, delta); err != nil {
			return fmt.Errorf("adjust doc_freq: %w", err)
		}
		delete(w.docFreqDelta, valueID)
	}
	return nil
}

// FlushWorkingSet persists pending document-frequency changes and releases
// cached keyword identifiers. The surrounding transaction remains open.
func (w *PutExecutor) FlushWorkingSet(ctx context.Context) error {
	if err := w.FlushDocFreq(ctx); err != nil {
		return err
	}
	clear(w.keywordIDs)
	return nil
}

func (w *PutExecutor) keywordID(ctx context.Context, field, value string) (int64, error) {
	key := keywordCacheKey{field: field, value: value}
	if valueID, ok := w.keywordIDs[key]; ok {
		return valueID, nil
	}
	valueID, err := insertKeyword(ctx, w.exec, w.sqlt, field, value)
	if err != nil {
		return 0, err
	}
	w.keywordIDs[key] = valueID
	return valueID, nil
}

func insertOrUpdateItem(ctx context.Context, exec storage.SQLExecutor, sqlt storage.SQL, path string, dataJSON []byte, nowMS int64) (itemID int64, createdAtMS int64, isNew bool, err error) {
	insertSQL, insertArgs := sqlt.UpsertItem.BuildInsert(path, dataJSON, nowMS, nowMS)
	err = scanOne(ctx, exec, insertSQL, insertArgs, &itemID, &createdAtMS)
	if err == nil {
		return itemID, createdAtMS, true, nil
	}
	if err != sql.ErrNoRows {
		return 0, 0, false, err
	}

	sql, args := sqlt.UpsertItem.Build(path, dataJSON, nowMS, nowMS, false)
	if err := scanOne(ctx, exec, sql, args, &itemID, &createdAtMS); err != nil {
		return 0, 0, false, err
	}
	return itemID, createdAtMS, false, nil
}

func loadOldValueIDs(ctx context.Context, exec storage.SQLExecutor, sqlt storage.SQL, itemID int64) (map[int64]bool, error) {
	result := make(map[int64]bool)
	rows, err := exec.QueryContext(ctx, sqlt.GetValueIDsByItem, itemID)
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

func deleteOldIndexRows(ctx context.Context, exec storage.SQLExecutor, sqlt storage.SQL, fts storage.FTS, itemID int64) error {
	// Delete in specific order to avoid FK issues
	queries := []string{
		sqlt.DeletePostingsByItem,
		sqlt.DeleteNumberByItem,
		sqlt.DeleteDateByItem,
		sqlt.DeleteBoolByItem,
		sqlt.DeletePresentByItem,
	}

	for _, q := range queries {
		if _, err := exec.ExecContext(ctx, q, itemID); err != nil {
			return err
		}
	}

	// Delete FTS row (handled specially by FTS driver)
	if err := fts.DeleteRow(ctx, exec, itemID); err != nil {
		return err
	}

	return nil
}

func insertKeyword(ctx context.Context, exec storage.SQLExecutor, sqlt storage.SQL, field, value string) (int64, error) {
	// Insert or ignore into dict
	if _, err := exec.ExecContext(ctx, sqlt.InsertOrIgnoreKwDict, field, value); err != nil {
		return 0, err
	}

	// Get dict ID
	var valueID int64
	err := scanOne(ctx, exec, sqlt.GetKwDictID, []any{field, value}, &valueID)
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
