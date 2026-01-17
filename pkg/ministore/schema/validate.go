package schema

import (
	"regexp"

	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
)

var fieldNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s Schema) Validate() error {
	if len(s.Fields) == 0 {
		return mserrors.NewError(mserrors.ErrSchema, "schema must have at least one field")
	}
	for name, spec := range s.Fields {
		if !fieldNameRe.MatchString(name) {
			return mserrors.NewError(mserrors.ErrSchema, "invalid field name: "+name)
		}
		switch name {
		case "path", "created", "updated":
			return mserrors.NewError(mserrors.ErrSchema, "reserved field name: "+name)
		}
		if spec.Weight != nil {
			if spec.Type != FieldText {
				return mserrors.NewError(mserrors.ErrSchema, "weight only allowed for text: "+name)
			}
			if *spec.Weight <= 0 {
				return mserrors.NewError(mserrors.ErrSchema, "weight must be > 0: "+name)
			}
		}
	}
	return nil
}
