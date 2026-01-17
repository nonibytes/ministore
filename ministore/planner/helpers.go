package planner

import (
	"fmt"
	"strings"
	"time"
)

// literalPrefixBeforeWildcard returns the literal part before the first wildcard
func literalPrefixBeforeWildcard(pattern string) string {
	for i, c := range pattern {
		if c == '*' || c == '?' {
			return pattern[:i]
		}
	}
	return pattern
}

// parseDateToEpochMS parses a date string to epoch milliseconds
func parseDateToEpochMS(s string) (int64, error) {
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

// quoteFTSTerm quotes an FTS term if it contains special characters
func quoteFTSTerm(term string) string {
	needsQuote := false
	for _, c := range term {
		switch {
		case c == '"' || c == ':' || c == '*' || c == '(' || c == ')' || c == '^':
			needsQuote = true
		case c <= ' ':
			needsQuote = true
		}
		if needsQuote {
			break
		}
	}
	if !needsQuote {
		return term
	}
	// Escape internal quotes by doubling them
	escaped := strings.ReplaceAll(term, `"`, `""`)
	return fmt.Sprintf(`"%s"`, escaped)
}

// joinOr joins strings with " OR "
func joinOr(parts []string) string {
	return strings.Join(parts, " OR ")
}
