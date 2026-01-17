package schema

import (
	"bytes"
	"encoding/json"
	"sort"

	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
)

// ToCanonicalJSON returns stable JSON so cursor hashing is consistent.
func (s Schema) ToCanonicalJSON() ([]byte, error) {
	// Because Go maps are randomized, we re-encode with sorted keys.
	// This is a minimal canonicalization for hashing/metadata; adapters may store raw schema too.
	m := make(map[string]any, 2)
	m["fields"] = s.Fields
	if len(s.TextOrder) > 0 {
		m["text_order"] = s.TextOrder
	}
	// Encode fields with sorted keys.
	// We do this by building an intermediate ordered representation.
	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([]any, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, map[string]any{"name": k, "spec": s.Fields[k]})
	}
	m["__ordered_fields"] = ordered
	b, err := json.Marshal(m)
	if err != nil {
		return nil, mserrors.Wrap(mserrors.ErrSchema, "json encode", err)
	}
	return bytes.TrimSpace(b), nil
}

func FromJSON(b []byte) (Schema, error) {
	var s Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return Schema{}, mserrors.Wrap(mserrors.ErrSchema, "json parse", err)
	}
	if s.Fields == nil {
		s.Fields = map[string]FieldSpec{}
	}
	return s, nil
}
