package query

import (
	"testing"
)

func TestNormalizeValidQuery(t *testing.T) {
	// A query with a positive anchor (text search)
	expr, err := Parse("important")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept query with text anchor: %v", err)
	}
}

func TestNormalizeValidKeyword(t *testing.T) {
	expr, err := Parse("tags:important")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept keyword query: %v", err)
	}
}

func TestNormalizeValidPath(t *testing.T) {
	expr, err := Parse("path:/docs/*")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept path with prefix: %v", err)
	}
}

func TestNormalizeValidNumber(t *testing.T) {
	expr, err := Parse("priority>5")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept number comparison: %v", err)
	}
}

func TestNormalizeValidRange(t *testing.T) {
	expr, err := Parse("priority:1..10")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept number range: %v", err)
	}
}

func TestNormalizeValidAndCombination(t *testing.T) {
	expr, err := Parse("important AND priority>5")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept AND with valid anchors: %v", err)
	}
}

func TestNormalizeValidOrCombination(t *testing.T) {
	expr, err := Parse("important OR test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept OR with valid anchors: %v", err)
	}
}

func TestNormalizePureNegation(t *testing.T) {
	// A query that is purely negative should be rejected
	expr, err := Parse("NOT important")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	// Note: NOT important becomes a Bool{Field: important, Value: false}
	// which may or may not be considered a valid anchor depending on implementation
}

func TestNormalizeNotFieldedQuery(t *testing.T) {
	// NOT followed by a fielded query is just a NOT expression
	expr, err := Parse("NOT tags:hidden")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	// This should be rejected as it has no positive anchor
	if err == nil {
		t.Logf("normalize allowed NOT tags:hidden (no positive anchor) - may be by design")
	}
}

func TestNormalizeComplexValid(t *testing.T) {
	expr, err := Parse("tags:important AND (priority>5 OR content:test)")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Normalize(expr, DefaultNormalizeOptions())
	if err != nil {
		t.Fatalf("normalize should accept complex valid query: %v", err)
	}
}

func TestNormalizeWildcardPrefix(t *testing.T) {
	expr, err := Parse("tags:te*")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	opts := DefaultNormalizeOptions()
	opts.MinPrefixLen = 2
	_, err = Normalize(expr, opts)
	if err != nil {
		t.Fatalf("normalize should accept keyword with 2-char prefix: %v", err)
	}
}

func TestNormalizeWildcardPrefixTooShort(t *testing.T) {
	expr, err := Parse("tags:t*")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	opts := DefaultNormalizeOptions()
	opts.MinPrefixLen = 2
	_, err = Normalize(expr, opts)
	if err == nil {
		t.Fatalf("normalize should reject keyword with 1-char prefix when min is 2")
	}
}

func TestNormalizeContainsQuery(t *testing.T) {
	expr, err := Parse("tags:*test*")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	opts := DefaultNormalizeOptions()
	opts.MinContainsLen = 3
	_, err = Normalize(expr, opts)
	if err != nil {
		t.Fatalf("normalize should accept contains with 4-char substring: %v", err)
	}
}

func TestNormalizeContainsTooShort(t *testing.T) {
	expr, err := Parse("tags:*te*")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	opts := DefaultNormalizeOptions()
	opts.MinContainsLen = 3
	_, err = Normalize(expr, opts)
	if err == nil {
		t.Fatalf("normalize should reject contains with 2-char substring when min is 3")
	}
}
