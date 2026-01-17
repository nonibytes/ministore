package ministore

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/ministore/ministore/ministore/storage"
)

// FieldType specifies the type of a field
type FieldType string

const (
	FieldKeyword FieldType = "keyword"
	FieldText    FieldType = "text"
	FieldNumber  FieldType = "number"
	FieldDate    FieldType = "date"
	FieldBool    FieldType = "bool"
)

// FieldSpec defines a field's configuration
type FieldSpec struct {
	Type   FieldType `json:"type"`
	Multi  bool      `json:"multi,omitempty"`
	Weight *float64  `json:"weight,omitempty"` // text fields only
}

// Schema defines the structure of an index
type Schema struct {
	Fields map[string]FieldSpec `json:"fields"`
}

var validFieldNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var reservedFieldNames = map[string]bool{
	"path":    true,
	"created": true,
	"updated": true,
}

// Validate checks if the schema is valid
func (s Schema) Validate() error {
	if len(s.Fields) == 0 {
		return SchemaError("schema must have at least one field")
	}

	for name, spec := range s.Fields {
		if !validFieldNameRe.MatchString(name) {
			return SchemaError(fmt.Sprintf("invalid field name: %s (must match ^[A-Za-z_][A-Za-z0-9_]*$)", name))
		}
		if reservedFieldNames[name] {
			return SchemaError(fmt.Sprintf("field name '%s' is reserved", name))
		}

		switch spec.Type {
		case FieldKeyword, FieldText, FieldNumber, FieldDate, FieldBool:
			// valid
		default:
			return SchemaError(fmt.Sprintf("unknown field type '%s' for field '%s'", spec.Type, name))
		}

		if spec.Weight != nil {
			if spec.Type != FieldText {
				return SchemaError(fmt.Sprintf("field '%s': weight can only be specified for text fields", name))
			}
			if *spec.Weight <= 0 {
				return SchemaError(fmt.Sprintf("field '%s': weight must be positive", name))
			}
		}
	}

	return nil
}

// ToJSON serializes the schema to JSON
func (s Schema) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// SchemaFromJSON deserializes a schema from JSON
func SchemaFromJSON(b []byte) (Schema, error) {
	var s Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return Schema{}, Wrap(ErrSchema, "invalid schema JSON", err)
	}
	if err := s.Validate(); err != nil {
		return Schema{}, err
	}
	return s, nil
}

// TextField represents a text field with its weight
type TextField struct {
	Name   string
	Weight float64
}

// TextFieldsInOrder returns text fields sorted by name with their weights
func (s Schema) TextFieldsInOrder() []TextField {
	var fields []TextField
	for name, spec := range s.Fields {
		if spec.Type == FieldText {
			weight := 1.0
			if spec.Weight != nil {
				weight = *spec.Weight
			}
			fields = append(fields, TextField{Name: name, Weight: weight})
		}
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})
	return fields
}

// Get retrieves a field spec by name
func (s Schema) Get(name string) (FieldSpec, bool) {
	spec, ok := s.Fields[name]
	return spec, ok
}

// HasField checks if a field exists in the schema
func (s Schema) HasField(name string) bool {
	_, ok := s.Fields[name]
	return ok
}

// IsMulti checks if a field allows multiple values
func (s Schema) IsMulti(name string) bool {
	spec, ok := s.Fields[name]
	return ok && spec.Multi
}

// The following methods are needed to implement storage.Schema interface
// They convert between ministore types and storage types

// GetStorageFieldSpec retrieves a field spec as storage.FieldSpec
func (s *Schema) GetStorageFieldSpec(name string) (storage.FieldSpec, bool) {
	spec, ok := s.Fields[name]
	if !ok {
		return storage.FieldSpec{}, false
	}
	return storage.FieldSpec{
		Type:   storage.FieldType(spec.Type),
		Multi:  spec.Multi,
		Weight: spec.Weight,
	}, true
}

// TextFieldsInStorageFormat returns text fields in storage format
func (s *Schema) TextFieldsInStorageFormat() []storage.TextField {
	fields := s.TextFieldsInOrder()
	result := make([]storage.TextField, len(fields))
	for i, f := range fields {
		result[i] = storage.TextField{Name: f.Name, Weight: f.Weight}
	}
	return result
}

// schemaStorageAdapter wraps Schema to implement storage.Schema interface
type schemaStorageAdapter struct {
	*Schema
}

// ToJSON implements storage.Schema
func (s schemaStorageAdapter) ToJSON() ([]byte, error) {
	return s.Schema.ToJSON()
}

// TextFieldsInOrder implements storage.Schema
func (s schemaStorageAdapter) TextFieldsInOrder() []storage.TextField {
	return s.Schema.TextFieldsInStorageFormat()
}

// Get implements storage.Schema
func (s schemaStorageAdapter) Get(name string) (storage.FieldSpec, bool) {
	return s.Schema.GetStorageFieldSpec(name)
}

// HasField implements storage.Schema
func (s schemaStorageAdapter) HasField(name string) bool {
	return s.Schema.HasField(name)
}

// AsStorageSchema returns a storage.Schema adapter
func (s *Schema) AsStorageSchema() storage.Schema {
	return schemaStorageAdapter{s}
}
