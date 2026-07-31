package okf

// Severity identifies whether a finding prevents base conformance or provides
// advisory guidance.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// FindingCode is a stable, versioned OKF diagnostic identifier.
type FindingCode string

const (
	CodeInvalidUTF8             FindingCode = "OKF100"
	CodeMissingOpeningDelimiter FindingCode = "OKF101"
	CodeMissingClosingDelimiter FindingCode = "OKF102"
	CodeInvalidYAML             FindingCode = "OKF103"
	CodeFrontmatterNotMapping   FindingCode = "OKF104"
	CodeMissingType             FindingCode = "OKF105"
	CodeDuplicateKey            FindingCode = "OKF106"
	CodeUTF8BOM                 FindingCode = "OKF107"
	CodeDelimiterWhitespace     FindingCode = "OKF108"
	CodeNestedIndexFrontmatter  FindingCode = "OKF200"
	CodeMalformedIndex          FindingCode = "OKF201"
	CodeIndexEntryNotLinkFirst  FindingCode = "OKF202"
	CodeMalformedLogDate        FindingCode = "OKF203"
	CodeCaseFoldCollision       FindingCode = "OKF204"
	CodeIgnoredSpecialFile      FindingCode = "OKF205"
	CodeInvalidVersion          FindingCode = "OKF206"
	CodeUnsupportedVersion      FindingCode = "OKF207"
	CodeMalformedSources        FindingCode = "OKF300"
	CodeSourceMissingResource   FindingCode = "OKF301"
	CodeDuplicateSourceID       FindingCode = "OKF302"
	CodeMalformedUsageWindow    FindingCode = "OKF303"
	CodeMalformedCredibility    FindingCode = "OKF304"
	CodeMalformedGenerated      FindingCode = "OKF310"
	CodeMalformedGeneratedAt    FindingCode = "OKF311"
	CodeMalformedVerified       FindingCode = "OKF312"
	CodeMalformedVerification   FindingCode = "OKF313"
	CodeMalformedActor          FindingCode = "OKF314"
	CodeVerificationPredatesGen FindingCode = "OKF315"
	CodeMalformedStatus         FindingCode = "OKF320"
	CodeMalformedStaleAfter     FindingCode = "OKF321"
	CodeMalformedTags           FindingCode = "OKF330"
	CodeLegacyTimestamp         FindingCode = "OKF340"
	CodeLegacyCitations         FindingCode = "OKF341"
	CodeMissingRuntime          FindingCode = "OKF350"
	CodeMalformedParameters     FindingCode = "OKF351"
	CodeMalformedComputation    FindingCode = "OKF352"
	CodeMalformedExecutor       FindingCode = "OKF353"
	CodeMalformedAttester       FindingCode = "OKF354"
	CodeUnmatchedFootnote       FindingCode = "OKF360"
	CodeMissingLinkTarget       FindingCode = "OKF400"
	CodeLinkEscapesRoot         FindingCode = "OKF401"
	CodeUnsafePercentEncoding   FindingCode = "OKF402"
)

// Finding is one deterministic conformance or advisory diagnostic.
type Finding struct {
	Severity    Severity    `json:"severity"`
	Code        FindingCode `json:"code"`
	Path        string      `json:"path"`
	Line        *int        `json:"line,omitempty"`
	Column      *int        `json:"column,omitempty"`
	SpecSection string      `json:"spec_section,omitempty"`
	Message     string      `json:"message"`
}

func position(value int) *int {
	return &value
}
