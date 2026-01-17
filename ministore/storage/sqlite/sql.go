package sqlite

import "github.com/ministore/ministore/ministore/storage"

type upsertItem struct {
	withTimestamps bool
}

func (u upsertItem) Build(path string, dataJSON []byte, createdAtMS, updatedAtMS int64, nowMode bool) (string, []any) {
	c := createdAtMS
	uMs := updatedAtMS
	if nowMode {
		c = updatedAtMS
	}
	if u.withTimestamps {
		sql := `INSERT INTO items(path, data_json, created_at, updated_at)
			VALUES(?1, ?2, ?3, ?4)
			ON CONFLICT(path) DO UPDATE SET data_json=excluded.data_json, created_at=excluded.created_at, updated_at=excluded.updated_at
			RETURNING id, created_at`
		return sql, []any{path, string(dataJSON), c, uMs}
	}
	sql := `INSERT INTO items(path, data_json, created_at, updated_at)
		VALUES(?1, ?2, ?3, ?4)
		ON CONFLICT(path) DO UPDATE SET data_json=excluded.data_json, updated_at=excluded.updated_at
		RETURNING id, created_at`
	return sql, []any{path, string(dataJSON), c, uMs}
}

var SQLTemplates = storage.SQL{
	GetMeta:                   "SELECT value FROM meta WHERE key = ?1",
	SetMeta:                   "INSERT INTO meta(key,value) VALUES(?1,?2) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
	FindItemIDByPath:          "SELECT id, created_at FROM items WHERE path = ?1",
	GetItemByPath:             "SELECT id, data_json, created_at, updated_at FROM items WHERE path = ?1",
	CleanupExpiredCursors:     "DELETE FROM cursor_store WHERE expires_at < ?1",
	GetCursor:                 "SELECT payload, expires_at FROM cursor_store WHERE handle = ?1",
	PutCursor:                 "INSERT INTO cursor_store(handle, payload, created_at, expires_at) VALUES(?1,?2,?3,?4)",
	GetValueIDsByItem:         "SELECT value_id FROM kw_postings WHERE item_id = ?1",
	DecrementDocFreq:          "UPDATE kw_dict SET doc_freq = CASE WHEN doc_freq > 0 THEN doc_freq - 1 ELSE 0 END WHERE id = ?1",
	IncrementDocFreq:          "UPDATE kw_dict SET doc_freq = doc_freq + 1 WHERE id = ?1",
	DeleteSearchRow:           "DELETE FROM search WHERE rowid = ?1",
	DeletePresentByItem:       "DELETE FROM field_present WHERE item_id = ?1",
	DeletePostingsByItem:      "DELETE FROM kw_postings WHERE item_id = ?1",
	DeleteNumberByItem:        "DELETE FROM field_number WHERE item_id = ?1",
	DeleteDateByItem:          "DELETE FROM field_date WHERE item_id = ?1",
	DeleteBoolByItem:          "DELETE FROM field_bool WHERE item_id = ?1",
	DeleteItemsByID:           "DELETE FROM items WHERE id = ?1",
	InsertOrIgnoreKwDict:      "INSERT OR IGNORE INTO kw_dict(field, value, doc_freq) VALUES(?1, ?2, 0)",
	GetKwDictID:               "SELECT id FROM kw_dict WHERE field = ?1 AND value = ?2",
	InsertOrIgnoreKwPosting:   "INSERT OR IGNORE INTO kw_postings(field, value_id, item_id) VALUES(?1, ?2, ?3)",
	InsertFieldPresent:        "INSERT INTO field_present(item_id, field) VALUES(?1, ?2)",
	InsertFieldNumber:         "INSERT INTO field_number(item_id, field, value) VALUES(?1, ?2, ?3)",
	InsertFieldDate:           "INSERT INTO field_date(item_id, field, value) VALUES(?1, ?2, ?3)",
	InsertFieldBool:           "INSERT INTO field_bool(item_id, field, value) VALUES(?1, ?2, ?3)",
	UpsertItem:                upsertItem{withTimestamps: false},
	UpsertItemWithTS:          upsertItem{withTimestamps: true},
}
