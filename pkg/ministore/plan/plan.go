package plan

import (
	"github.com/nonibytes/ministore/pkg/ministore/query"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
)

type CompiledQuery struct {
	Expr         query.Expr
	Schema       schema.Schema
	NowMs        int64
	ExplainSteps []string
	RequiresText bool
}
