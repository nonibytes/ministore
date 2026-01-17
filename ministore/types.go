package ministore

import "time"

// CursorMode specifies how cursors are returned
type CursorMode string

const (
	CursorShort CursorMode = "short" // c:handle stored in DB
	CursorFull  CursorMode = "full"  // self-contained base64url JSON
)

// RankModeKind specifies the type of ranking
type RankModeKind string

const (
	RankDefault RankModeKind = "default" // FTS relevance or fallback to recency
	RankRecency RankModeKind = "recency" // updated_at DESC
	RankField   RankModeKind = "field"   // Sort by numeric/date field
	RankNone    RankModeKind = "none"    // Insertion order (item_id)
)

// RankMode configures result ranking
type RankMode struct {
	Kind  RankModeKind
	Field string // only used when Kind==RankField
}

// OutputFieldSelectorKind specifies which fields to include in output
type OutputFieldSelectorKind string

const (
	ShowNone   OutputFieldSelectorKind = "none"   // Only path
	ShowAll    OutputFieldSelectorKind = "all"    // All fields
	ShowFields OutputFieldSelectorKind = "fields" // Specified fields
)

// OutputFieldSelector configures which fields are included in search results
type OutputFieldSelector struct {
	Kind   OutputFieldSelectorKind
	Fields []string // only used when Kind==ShowFields
}

// IndexOptions configures index behavior
type IndexOptions struct {
	CursorTTL          time.Duration // default 1h
	Now                func() time.Time
	MinContainsLen     int
	MinPrefixLen       int
	MaxPrefixExpansion int
}

// DefaultIndexOptions returns sensible defaults
func DefaultIndexOptions() IndexOptions {
	return IndexOptions{
		CursorTTL:          DefaultCursorTTL,
		Now:                time.Now,
		MinContainsLen:     DefaultMinContainsLen,
		MinPrefixLen:       DefaultMinPrefixLen,
		MaxPrefixExpansion: DefaultMaxPrefixExpansion,
	}
}

// SearchOptions configures a search operation
type SearchOptions struct {
	Rank       RankMode
	Limit      int
	After      string // cursor token or ""
	CursorMode CursorMode
	Show       OutputFieldSelector
	Explain    bool
}

// ItemMeta holds item metadata
type ItemMeta struct {
	CreatedAtMS int64
	UpdatedAtMS int64
}

// ItemView is a complete item with metadata
type ItemView struct {
	Path    string
	DocJSON []byte
	Meta    ItemMeta
}

// SearchResultPage is a page of search results
type SearchResultPage struct {
	Items        [][]byte // output-shaped JSON per item
	NextCursor   string
	HasMore      bool
	ExplainSQL   string
	ExplainSteps []string
}

// ValueCount is a field value with count
type ValueCount struct {
	Value string
	Count uint64
}

// FieldOverview describes a field's statistics
type FieldOverview struct {
	Field    string
	Type     FieldType
	Multi    bool
	DocCount uint64
	Unique   *uint64
	Weight   *float64
	Examples []string
}

// StatsResult contains aggregated statistics for a field
type StatsResult struct {
	Field  string
	Count  uint64
	Min    *float64
	Max    *float64
	Avg    *float64
	Median *float64
}
