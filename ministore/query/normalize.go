package query

import (
	"fmt"
	"strings"
)

// NormalizeOptions configures normalization guardrails
type NormalizeOptions struct {
	MinContainsLen     int
	MinPrefixLen       int
	MaxPrefixExpansion int
}

// DefaultNormalizeOptions returns default normalization options
func DefaultNormalizeOptions() NormalizeOptions {
	return NormalizeOptions{
		MinContainsLen:     3,
		MinPrefixLen:       2,
		MaxPrefixExpansion: 20000,
	}
}

// Normalize validates and normalizes a parsed expression
// It enforces positive anchors and guardrails
func Normalize(expr Expr, opts NormalizeOptions) (Expr, error) {
	// Check for positive anchor
	if !hasPositiveAnchor(expr) {
		return nil, fmt.Errorf("query must have at least one positive anchor (text search, exact keyword match, numeric/date predicate, or path with literal prefix)")
	}

	// Validate guardrails
	if err := validateGuardrails(expr, opts); err != nil {
		return nil, err
	}

	return expr, nil
}

// hasPositiveAnchor checks if the expression contains at least one positive anchor
func hasPositiveAnchor(expr Expr) bool {
	switch e := expr.(type) {
	case And:
		return hasPositiveAnchor(e.Left) || hasPositiveAnchor(e.Right)
	case Or:
		return hasPositiveAnchor(e.Left) && hasPositiveAnchor(e.Right) // both branches need anchors
	case Not:
		return false // NOT is not a positive anchor
	case Pred:
		return predicateIsAnchor(e.Predicate)
	default:
		return false
	}
}

// predicateIsAnchor returns true if the predicate can serve as a positive anchor
func predicateIsAnchor(pred Predicate) bool {
	switch p := pred.(type) {
	case Text:
		return true // FTS is always an anchor
	case Keyword:
		// Exact match is an anchor
		// Prefix/contains/glob need literal prefix to be anchors
		switch p.Kind {
		case KeywordExact:
			return true
		case KeywordPrefix:
			prefix := strings.TrimSuffix(p.Pattern, "*")
			return len(prefix) >= 2 // needs at least 2 chars
		case KeywordContains:
			inner := strings.Trim(p.Pattern, "*")
			return len(inner) >= 3 // needs at least 3 chars
		case KeywordGlob:
			// Find literal prefix before first wildcard
			prefix := literalPrefixBeforeWildcard(p.Pattern)
			return len(prefix) >= 2
		}
	case NumberCmp, NumberRange:
		return true
	case DateCmpAbs, DateRangeAbs, DateCmpRel:
		return true
	case Bool:
		return true
	case PathGlob:
		// Path with literal prefix is an anchor
		prefix := literalPrefixBeforeWildcard(p.Pattern)
		return len(prefix) >= 1 // even "/" is enough
	case Has:
		return true // field presence is an anchor
	}
	return false
}

// validateGuardrails checks that patterns meet minimum length requirements
func validateGuardrails(expr Expr, opts NormalizeOptions) error {
	switch e := expr.(type) {
	case And:
		if err := validateGuardrails(e.Left, opts); err != nil {
			return err
		}
		return validateGuardrails(e.Right, opts)
	case Or:
		if err := validateGuardrails(e.Left, opts); err != nil {
			return err
		}
		return validateGuardrails(e.Right, opts)
	case Not:
		return validateGuardrails(e.Inner, opts)
	case Pred:
		return validatePredicateGuardrails(e.Predicate, opts)
	}
	return nil
}

func validatePredicateGuardrails(pred Predicate, opts NormalizeOptions) error {
	switch p := pred.(type) {
	case Keyword:
		switch p.Kind {
		case KeywordPrefix:
			prefix := strings.TrimSuffix(p.Pattern, "*")
			if len(prefix) < opts.MinPrefixLen {
				return fmt.Errorf("prefix pattern '%s' too short (min %d characters before *)", p.Pattern, opts.MinPrefixLen)
			}
		case KeywordContains:
			inner := strings.Trim(p.Pattern, "*")
			if len(inner) < opts.MinContainsLen {
				return fmt.Errorf("contains pattern '%s' too short (min %d characters)", p.Pattern, opts.MinContainsLen)
			}
		case KeywordGlob:
			prefix := literalPrefixBeforeWildcard(p.Pattern)
			if len(prefix) < opts.MinPrefixLen {
				return fmt.Errorf("glob pattern '%s' needs literal prefix of at least %d characters", p.Pattern, opts.MinPrefixLen)
			}
		}
	case PathGlob:
		// Check for reasonable prefix
		prefix := literalPrefixBeforeWildcard(p.Pattern)
		if len(prefix) == 0 {
			return fmt.Errorf("path pattern '%s' needs literal prefix before wildcard", p.Pattern)
		}
	case Text:
		if len(p.FTS) == 0 {
			return fmt.Errorf("text search term cannot be empty")
		}
	}
	return nil
}

// literalPrefixBeforeWildcard returns the literal part before the first wildcard
func literalPrefixBeforeWildcard(pattern string) string {
	for i, c := range pattern {
		if c == '*' || c == '?' {
			return pattern[:i]
		}
	}
	return pattern
}

// ExtractTextPredicates collects all Text predicates from an expression
func ExtractTextPredicates(expr Expr) []Text {
	var result []Text
	extractTextPredicatesInto(expr, &result)
	return result
}

func extractTextPredicatesInto(expr Expr, result *[]Text) {
	switch e := expr.(type) {
	case And:
		extractTextPredicatesInto(e.Left, result)
		extractTextPredicatesInto(e.Right, result)
	case Or:
		extractTextPredicatesInto(e.Left, result)
		extractTextPredicatesInto(e.Right, result)
	case Not:
		extractTextPredicatesInto(e.Inner, result)
	case Pred:
		if text, ok := e.Predicate.(Text); ok {
			*result = append(*result, text)
		}
	}
}
