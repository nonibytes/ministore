package okf

// ValidateOptions controls bundle validation behavior.
type ValidateOptions struct {
	TargetVersion string
}

type SyncOptions struct {
	TargetVersion string
	Strict        bool
	DryRun        bool
}

type SyncReport struct {
	OK                bool              `json:"ok"`
	Bundle            string            `json:"bundle"`
	ProjectionVersion int               `json:"projection_version"`
	Concepts          int               `json:"concepts"`
	Added             int               `json:"added"`
	Updated           int               `json:"updated"`
	Unchanged         int               `json:"unchanged"`
	Deleted           int               `json:"deleted"`
	DurationMS        int64             `json:"duration_ms"`
	Validation        ValidationSummary `json:"validation"`
}

// FindingSink receives findings in deterministic order.
type FindingSink func(Finding) error

// Projection is the canonical flat MiniStore document for one OKF concept.
type Projection map[string]any

// ProjectionSink receives projections in source-path order.
type ProjectionSink func(Projection) error

// ValidationSummary contains disk-backed bundle validation counts.
type ValidationSummary struct {
	TargetVersion   string  `json:"target_version"`
	DeclaredVersion *string `json:"declared_version"`
	Bundle          string  `json:"bundle"`
	Concepts        int     `json:"concepts"`
	Errors          int     `json:"errors"`
	Warnings        int     `json:"warnings"`
}

// OK reports whether the bundle has no base-conformance errors.
func (s ValidationSummary) OK() bool {
	return s.Errors == 0
}
