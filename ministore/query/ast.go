package query

// Expr represents a query expression
type Expr interface {
	isExpr()
}

// And represents a boolean AND of two expressions
type And struct {
	Left  Expr
	Right Expr
}

func (And) isExpr() {}

// Or represents a boolean OR of two expressions
type Or struct {
	Left  Expr
	Right Expr
}

func (Or) isExpr() {}

// Not represents a boolean NOT of an expression
type Not struct {
	Inner Expr
}

func (Not) isExpr() {}

// Pred wraps a predicate as an expression
type Pred struct {
	Predicate Predicate
}

func (Pred) isExpr() {}

// Predicate represents a search predicate
type Predicate interface {
	isPredicate()
}

// Has checks if a field exists
type Has struct {
	Field string
}

func (Has) isPredicate() {}

// PathGlob matches items by path pattern
type PathGlob struct {
	Pattern string
}

func (PathGlob) isPredicate() {}

// KeywordPatternKind indicates the type of keyword pattern
type KeywordPatternKind int

const (
	KeywordExact KeywordPatternKind = iota
	KeywordPrefix
	KeywordContains
	KeywordGlob
)

// Keyword matches a keyword field
type Keyword struct {
	Field   string
	Pattern string
	Kind    KeywordPatternKind
}

func (Keyword) isPredicate() {}

// Text performs full-text search
type Text struct {
	Field *string // nil means search all text fields
	FTS   string
}

func (Text) isPredicate() {}

// CmpOp is a comparison operator
type CmpOp int

const (
	CmpEq CmpOp = iota
	CmpGt
	CmpGte
	CmpLt
	CmpLte
)

func (op CmpOp) String() string {
	switch op {
	case CmpEq:
		return "="
	case CmpGt:
		return ">"
	case CmpGte:
		return ">="
	case CmpLt:
		return "<"
	case CmpLte:
		return "<="
	default:
		return "?"
	}
}

// NumberCmp compares a numeric field
type NumberCmp struct {
	Field string
	Op    CmpOp
	Value float64
}

func (NumberCmp) isPredicate() {}

// NumberRange matches a numeric field within a range (inclusive)
type NumberRange struct {
	Field string
	Lo    float64
	Hi    float64
}

func (NumberRange) isPredicate() {}

// DateCmpAbs compares a date field to an absolute timestamp
type DateCmpAbs struct {
	Field   string
	Op      CmpOp
	EpochMS int64
}

func (DateCmpAbs) isPredicate() {}

// DateRangeAbs matches a date field within an absolute range (inclusive)
type DateRangeAbs struct {
	Field string
	LoMS  int64
	HiMS  int64
}

func (DateRangeAbs) isPredicate() {}

// RelUnit is a relative time unit
type RelUnit int

const (
	RelH RelUnit = iota // hours
	RelD                // days
	RelW                // weeks
	RelM                // months
	RelY                // years
)

func (u RelUnit) String() string {
	switch u {
	case RelH:
		return "h"
	case RelD:
		return "d"
	case RelW:
		return "w"
	case RelM:
		return "m"
	case RelY:
		return "y"
	default:
		return "?"
	}
}

// ToMillis converts an amount and unit to milliseconds
func (u RelUnit) ToMillis(amount int64) int64 {
	switch u {
	case RelH:
		return amount * 3600 * 1000
	case RelD:
		return amount * 24 * 3600 * 1000
	case RelW:
		return amount * 7 * 24 * 3600 * 1000
	case RelM:
		return amount * 30 * 24 * 3600 * 1000 // approximate
	case RelY:
		return amount * 365 * 24 * 3600 * 1000 // approximate
	default:
		return 0
	}
}

// DateCmpRel compares a date field to a relative time offset
type DateCmpRel struct {
	Field  string
	Op     CmpOp
	Amount int64
	Unit   RelUnit
}

func (DateCmpRel) isPredicate() {}

// Bool matches a boolean field
type Bool struct {
	Field string
	Value bool
}

func (Bool) isPredicate() {}
