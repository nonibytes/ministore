package query

type ExprKind int

const (
	ExprAnd ExprKind = iota
	ExprOr
	ExprNot
	ExprPred
)

type Expr struct {
	Kind ExprKind
	Left  *Expr
	Right *Expr
	Pred  *Predicate
}

type PredicateKind int

const (
	PredHas PredicateKind = iota
	PredPathGlob
	PredKeyword
	PredText
	PredNumberCmp
	PredNumberRange
	PredDateAbsCmp
	PredDateAbsRange
	PredDateRelCmp
	PredBool
)

type KeywordPatternKind int

const (
	KwExact KeywordPatternKind = iota
	KwPrefix
	KwContains
	KwGlob
)

type CmpOp int

const (
	CmpEq CmpOp = iota
	CmpGt
	CmpGte
	CmpLt
	CmpLte
)

type RelUnit int

const (
	RelH RelUnit = iota
	RelD
	RelW
	RelM
	RelY
)

type Predicate struct {
	Kind PredicateKind

	Field   string
	Pattern string
	KwKind  KeywordPatternKind

	TextField *string
	FTS       string

	Op  CmpOp
	Num float64
	Lo  float64
	Hi  float64

	EpochMs int64
	LoMs    int64
	HiMs    int64
	Amount  int64
	Unit    RelUnit

	Bool bool
}
