package backend

type Capabilities struct {
	FullText bool

	KeywordExact    bool
	KeywordPrefix   bool
	KeywordContains bool
	KeywordGlob     bool

	PathPrefix bool
	PathGlob   bool
	HasPredicate bool

	NumberOps bool
	DateOps   bool
	BoolOps   bool

	RankDefault bool
	RankRecency bool
	RankField   bool
	RankNone    bool

	CursorShort bool
	CursorFull  bool

	DiscoverValues   bool
	StatsAggregations bool
}
