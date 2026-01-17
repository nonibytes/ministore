package ministore

import "encoding/json"

// unmarshalJSON is a helper to unmarshal JSON
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// marshalJSON is a helper to marshal JSON
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
