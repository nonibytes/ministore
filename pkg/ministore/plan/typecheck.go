package plan

import (
	"time"

	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
	"github.com/nonibytes/ministore/pkg/ministore/query"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
)

type CompileOptions struct {
	Now time.Time
}

// Compile performs schema-aware predicate resolution/coercion.
//
// Responsibilities:
// - unknown field detection
// - field:value resolution by schema type (text/bool/date/number/keyword)
// - implicit created/updated handling
// - relative date semantics mapping rules
func Compile(s schema.Schema, expr query.Expr, opts CompileOptions) (CompiledQuery, error) {
	return CompiledQuery{}, mserrors.Wrap(mserrors.ErrNotImpl, "compile", mserrors.ErrNotImplemented)
}
