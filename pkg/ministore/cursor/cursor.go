package cursor

import "github.com/nonibytes/ministore/pkg/ministore/types"

type PayloadKind string

const (
	KindFts     PayloadKind = "fts"
	KindRecency PayloadKind = "recency"
	KindField   PayloadKind = "field"
	KindNone    PayloadKind = "none"
	KindRedis   PayloadKind = "redis"
)

type Payload struct {
	Kind PayloadKind `json:"kind"`

	// FTS
	Score  float64 `json:"score,omitempty"`
	ItemID int64   `json:"item_id,omitempty"`

	// Recency
	UpdatedAtMs int64  `json:"updated_at_ms,omitempty"`
	Path        string `json:"path,omitempty"`

	// Field
	Field     string  `json:"field,omitempty"`
	RankValue float64 `json:"rank_value,omitempty"`

	// Redis cursor
	RedisCursorID int64  `json:"redis_cursor_id,omitempty"`
	RedisIndex    string `json:"redis_index,omitempty"`
}

type Position struct {
	Payload Payload        `json:"payload"`
	Hash    string         `json:"hash"`
	Rank    types.RankMode `json:"rank"`
}
