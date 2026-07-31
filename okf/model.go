package okf

// ValidateOptions controls bundle validation behavior.
type ValidateOptions struct {
	TargetVersion string
}

// FindingSink receives findings in deterministic order.
type FindingSink func(Finding) error

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
