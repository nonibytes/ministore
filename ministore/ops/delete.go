package ops

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ministore/ministore/ministore/storage"
)

// DeleteByItemID deletes an item and all its index entries by item ID
func DeleteByItemID(ctx context.Context, tx *sql.Tx, sqlt storage.SQL, fts storage.FTS, itemID int64) error {
	// 1. Load value_ids from postings for doc_freq maintenance
	valueIDs, err := loadOldValueIDs(ctx, tx, sqlt, itemID)
	if err != nil {
		return fmt.Errorf("load value_ids: %w", err)
	}

	// 2. Decrement doc_freq for each value_id (clamped at 0 for safety)
	for valueID := range valueIDs {
		if _, err := tx.ExecContext(ctx, sqlt.DecrementDocFreq, valueID); err != nil {
			return fmt.Errorf("decrement doc_freq: %w", err)
		}
	}

	// 3. Delete index rows
	queries := []struct {
		sql  string
		name string
	}{
		{sqlt.DeletePostingsByItem, "postings"},
		{sqlt.DeleteNumberByItem, "numbers"},
		{sqlt.DeleteDateByItem, "dates"},
		{sqlt.DeleteBoolByItem, "bools"},
		{sqlt.DeletePresentByItem, "present"},
	}

	for _, q := range queries {
		if _, err := tx.ExecContext(ctx, q.sql, itemID); err != nil {
			return fmt.Errorf("delete %s: %w", q.name, err)
		}
	}

	// 4. Delete FTS row
	if err := fts.DeleteRow(ctx, tx, itemID); err != nil {
		return fmt.Errorf("delete FTS: %w", err)
	}

	// 5. Delete items row
	if _, err := tx.ExecContext(ctx, sqlt.DeleteItemsByID, itemID); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	return nil
}

// DeleteByPath deletes an item by path, returns true if item was found and deleted
func DeleteByPath(ctx context.Context, db *sql.DB, sqlt storage.SQL, fts storage.FTS, path string) (bool, error) {
	// Find item_id
	var itemID int64
	var createdAt int64
	err := db.QueryRowContext(ctx, sqlt.FindItemIDByPath, path).Scan(&itemID, &createdAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find item: %w", err)
	}

	// Delete in transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := DeleteByItemID(ctx, tx, sqlt, fts, itemID); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	return true, nil
}

// DeleteWhere deletes all items matching a compiled query
// Returns the number of items deleted
func DeleteWhere(ctx context.Context, db *sql.DB, sqlt storage.SQL, fts storage.FTS, resultCTE string, cteParts []string, args []any) (int, error) {
	// Build the query to get item_ids
	var withClause string
	if len(cteParts) > 0 {
		withClause = "WITH " + joinComma(cteParts) + " "
	}
	selectSQL := fmt.Sprintf("%sSELECT item_id FROM %s", withClause, resultCTE)

	// Execute query to get all matching item_ids
	rows, err := db.QueryContext(ctx, selectSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var itemIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan item_id: %w", err)
		}
		itemIDs = append(itemIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate rows: %w", err)
	}

	if len(itemIDs) == 0 {
		return 0, nil
	}

	// Delete each item in a transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, itemID := range itemIDs {
		if err := DeleteByItemID(ctx, tx, sqlt, fts, itemID); err != nil {
			return 0, fmt.Errorf("delete item %d: %w", itemID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return len(itemIDs), nil
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
