package item

type ItemMeta struct {
	CreatedAtMs int64 `json:"created_at_ms"`
	UpdatedAtMs int64 `json:"updated_at_ms"`
}

type ItemView struct {
	Path string         `json:"path"`
	Doc  map[string]any `json:"doc"`
	Meta ItemMeta       `json:"meta"`
}
