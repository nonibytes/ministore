package index

import (
	"time"

	"github.com/nonibytes/ministore/pkg/ministore/types"
)

// Re-export the shared option types from pkg/ministore/types.
// Keeping these in index provides a nicer public API surface.

type IndexOptions = types.IndexOptions

type CursorMode = types.CursorMode

const (
	CursorShort CursorMode = types.CursorShort
	CursorFull  CursorMode = types.CursorFull
)

type RankMode = types.RankMode
type OutputFieldSelector = types.OutputFieldSelector
type SearchOptions = types.SearchOptions
type SearchResultPage = types.SearchResultPage

func ParseRankMode(s string) RankMode        { return types.ParseRankMode(s) }
func ParseShow(s string) OutputFieldSelector { return types.ParseShow(s) }

func DefaultIndexOptions() IndexOptions {
	return IndexOptions{CursorTTL: time.Hour}
}

func ParseCursorMode(s string) CursorMode {
	if s == string(CursorShort) {
		return CursorShort
	}
	return CursorFull
}
