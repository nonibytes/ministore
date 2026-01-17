package schema

type FieldType string

const (
	FieldKeyword FieldType = "keyword"
	FieldText    FieldType = "text"
	FieldNumber  FieldType = "number"
	FieldDate    FieldType = "date"
	FieldBool    FieldType = "bool"
)

type FieldSpec struct {
	Type   FieldType `json:"type"`
	Multi  bool      `json:"multi,omitempty"`
	Weight *float64  `json:"weight,omitempty"` // text only
}

type Schema struct {
	Fields map[string]FieldSpec `json:"fields"`

	// TextOrder is optional but recommended for stable FTS column ordering across additive schema changes.
	// If nil/empty, adapters may fall back to deterministic sorted order, but additive changes get trickier.
	TextOrder []string `json:"text_order,omitempty"`
}
