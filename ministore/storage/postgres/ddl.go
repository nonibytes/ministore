package postgres

const ddlBase = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT
);

CREATE TABLE IF NOT EXISTS items (
  id         BIGSERIAL PRIMARY KEY,
  path       TEXT UNIQUE NOT NULL,
  data_json  JSONB NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_items_path    ON items(path);
CREATE INDEX IF NOT EXISTS idx_items_updated ON items(updated_at);
CREATE INDEX IF NOT EXISTS idx_items_created ON items(created_at);

CREATE TABLE IF NOT EXISTS field_present (
  item_id BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  field   TEXT   NOT NULL,
  PRIMARY KEY (item_id, field)
);
CREATE INDEX IF NOT EXISTS idx_present_field ON field_present(field, item_id);

CREATE TABLE IF NOT EXISTS kw_dict (
  id       BIGSERIAL PRIMARY KEY,
  field    TEXT NOT NULL,
  value    TEXT NOT NULL,
  doc_freq BIGINT NOT NULL DEFAULT 0,
  UNIQUE (field, value)
);
CREATE INDEX IF NOT EXISTS idx_kw_dict_lookup ON kw_dict(field, value);

CREATE TABLE IF NOT EXISTS kw_postings (
  field    TEXT NOT NULL,
  value_id BIGINT NOT NULL REFERENCES kw_dict(id) ON DELETE CASCADE,
  item_id  BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  PRIMARY KEY (value_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_kw_postings_item  ON kw_postings(item_id);
CREATE INDEX IF NOT EXISTS idx_kw_postings_field ON kw_postings(field, value_id);

CREATE TABLE IF NOT EXISTS field_number (
  item_id BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  field   TEXT   NOT NULL,
  value   DOUBLE PRECISION NOT NULL,
  PRIMARY KEY (item_id, field, value)
);
CREATE INDEX IF NOT EXISTS idx_num_lookup ON field_number(field, value);

CREATE TABLE IF NOT EXISTS field_date (
  item_id BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  field   TEXT   NOT NULL,
  value   BIGINT NOT NULL,
  PRIMARY KEY (item_id, field, value)
);
CREATE INDEX IF NOT EXISTS idx_date_lookup ON field_date(field, value);

CREATE TABLE IF NOT EXISTS field_bool (
  item_id BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  field   TEXT   NOT NULL,
  value   SMALLINT NOT NULL,
  PRIMARY KEY (item_id, field)
);
CREATE INDEX IF NOT EXISTS idx_bool_lookup ON field_bool(field, value);

CREATE TABLE IF NOT EXISTS cursor_store (
  handle     TEXT PRIMARY KEY,
  payload    TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  expires_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cursor_expires ON cursor_store(expires_at);
`
