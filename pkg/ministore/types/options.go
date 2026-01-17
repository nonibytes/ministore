package types

import "time"

// IndexOptions configure index behavior that is consistent across backends.
type IndexOptions struct {
	CursorTTL time.Duration
}

// CursorMode controls whether cursors are stored server-side (short) or fully self-contained (full).
type CursorMode string

const (
	CursorShort CursorMode = "short"
	CursorFull  CursorMode = "full"
)

// RankMode selects ordering/ranking.
// Kind: "default"|"recency"|"field"|"none".
type RankMode struct {
	Kind  string
	Field string
}

// OutputFieldSelector controls which fields appear in Search output.
// Mode: "none"|"all"|"fields".
type OutputFieldSelector struct {
	Mode   string
	Fields []string
}

// SearchOptions are passed to backend stores for consistent pagination and ranking.
type SearchOptions struct {
	Rank       RankMode
	Limit      int
	After      string
	CursorMode CursorMode
	Show       OutputFieldSelector
	Explain    bool
}

// SearchResultPage is the backend-agnostic response returned from Search.
// Items are already shaped according to Show.
type SearchResultPage struct {
	Items        []map[string]any `json:"items"`
	NextCursor   string           `json:"next_cursor,omitempty"`
	HasMore      bool             `json:"has_more"`
	ExplainPlan  []string         `json:"explain_steps,omitempty"`
	ExplainQuery string           `json:"explain_query,omitempty"`
}

func ParseRankMode(s string) RankMode {
	switch {
	case s == "recency":
		return RankMode{Kind: "recency"}
	case s == "none":
		return RankMode{Kind: "none"}
	case len(s) > 6 && s[:6] == "field:":
		return RankMode{Kind: "field", Field: s[6:]}
	default:
		return RankMode{Kind: "default"}
	}
}

func ParseShow(s string) OutputFieldSelector {
	if s == "" {
		return OutputFieldSelector{Mode: "none"}
	}
	if s == "all" {
		return OutputFieldSelector{Mode: "all"}
	}
	// TODO: split by comma, trim spaces, validate field names.
	return OutputFieldSelector{Mode: "fields", Fields: []string{s}}
}
