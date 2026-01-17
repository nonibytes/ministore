package planner

import (
	"fmt"

	"github.com/ministore/ministore/ministore/query"
	"github.com/ministore/ministore/ministore/storage"
)

// CompileOutput is the result of compiling a query expression
type CompileOutput struct {
	CTEs            []CTE
	ResultCTE       string
	ExplainSteps    []string
	TextPreds       []TextPredicate
	RequiresFTSJoin bool
}

// CTE represents a Common Table Expression
type CTE struct {
	Name string
	SQL  string
}

// TextPredicate represents a text search predicate for scoring
type TextPredicate struct {
	Field *string
	Query string
}

// Compiler compiles query expressions to CTEs
type Compiler struct {
	schema          storage.Schema
	builder         storage.Builder
	nowMS           int64
	ctes            []CTE
	explainSteps    []string
	cteCounter      int
	textPreds       []TextPredicate
	requiresFTSJoin bool
}

// Compile compiles a query expression into CTEs
func Compile(schema storage.Schema, builder storage.Builder, expr query.Expr, nowMS int64) (*CompileOutput, error) {
	c := &Compiler{
		schema:  schema,
		builder: builder,
		nowMS:   nowMS,
	}

	resultCTE, err := c.compileExpr(expr)
	if err != nil {
		return nil, err
	}

	return &CompileOutput{
		CTEs:            c.ctes,
		ResultCTE:       resultCTE,
		ExplainSteps:    c.explainSteps,
		TextPreds:       c.textPreds,
		RequiresFTSJoin: c.requiresFTSJoin,
	}, nil
}

func (c *Compiler) nextCTEName() string {
	name := fmt.Sprintf("cte_%d", c.cteCounter)
	c.cteCounter++
	return name
}

func (c *Compiler) compileExpr(expr query.Expr) (string, error) {
	switch e := expr.(type) {
	case query.And:
		leftName, err := c.compileExpr(e.Left)
		if err != nil {
			return "", err
		}
		rightName, err := c.compileExpr(e.Right)
		if err != nil {
			return "", err
		}

		resultName := c.nextCTEName()
		sql := fmt.Sprintf("SELECT item_id FROM %s INTERSECT SELECT item_id FROM %s", leftName, rightName)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("INTERSECT %s AND %s", leftName, rightName))
		return resultName, nil

	case query.Or:
		leftName, err := c.compileExpr(e.Left)
		if err != nil {
			return "", err
		}
		rightName, err := c.compileExpr(e.Right)
		if err != nil {
			return "", err
		}

		resultName := c.nextCTEName()
		sql := fmt.Sprintf("SELECT item_id FROM %s UNION SELECT item_id FROM %s", leftName, rightName)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("UNION %s OR %s", leftName, rightName))
		return resultName, nil

	case query.Not:
		innerName, err := c.compileExpr(e.Inner)
		if err != nil {
			return "", err
		}

		resultName := c.nextCTEName()
		sql := fmt.Sprintf("SELECT id AS item_id FROM items EXCEPT SELECT item_id FROM %s", innerName)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("EXCEPT NOT %s", innerName))
		return resultName, nil

	case query.Pred:
		return c.compilePredicate(e.Predicate)

	default:
		return "", fmt.Errorf("unknown expression type: %T", expr)
	}
}

func (c *Compiler) compilePredicate(pred query.Predicate) (string, error) {
	switch p := pred.(type) {
	case query.Has:
		if !c.schema.HasField(p.Field) {
			return "", fmt.Errorf("unknown field: %s", p.Field)
		}
		resultName := c.nextCTEName()
		ph := c.builder.Arg(p.Field)
		sql := fmt.Sprintf("SELECT item_id FROM field_present WHERE field = %s", ph)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("HAS %s", p.Field))
		return resultName, nil

	case query.PathGlob:
		resultName := c.nextCTEName()
		var sql string

		// Prefix-only optimization: "/docs/*" => LIKE "/docs/%"
		pattern := p.Pattern
		prefix := literalPrefixBeforeWildcard(pattern)
		if pattern == prefix+"*" {
			// Pure prefix pattern
			ph := c.builder.Arg(prefix + "%")
			sql = fmt.Sprintf("SELECT id AS item_id FROM items WHERE path LIKE %s", ph)
		} else {
			// Use GLOB for more complex patterns
			ph := c.builder.Arg(pattern)
			sql = fmt.Sprintf("SELECT id AS item_id FROM items WHERE path GLOB %s", ph)
		}

		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("PATH %s", pattern))
		return resultName, nil

	case query.Keyword:
		return c.compileKeyword(p)

	case query.Text:
		return c.compileText(p)

	case query.NumberCmp:
		// Handle implicit created/updated fields (timestamps as numbers)
		if p.Field == "created" || p.Field == "updated" {
			resultName := c.nextCTEName()
			col := "created_at"
			if p.Field == "updated" {
				col = "updated_at"
			}
			ph := c.builder.Arg(int64(p.Value))
			sql := fmt.Sprintf("SELECT id AS item_id FROM items WHERE %s %s %s", col, p.Op.String(), ph)
			c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
			c.explainSteps = append(c.explainSteps, fmt.Sprintf("IMPLICIT TIMESTAMP %s%s%v", p.Field, p.Op.String(), p.Value))
			return resultName, nil
		}

		spec, ok := c.schema.Get(p.Field)
		if !ok {
			return "", fmt.Errorf("unknown field: %s", p.Field)
		}
		if spec.Type != storage.FieldType("number") {
			return "", fmt.Errorf("field %s is not a number field", p.Field)
		}

		resultName := c.nextCTEName()
		phField := c.builder.Arg(p.Field)
		phVal := c.builder.Arg(p.Value)
		sql := fmt.Sprintf("SELECT item_id FROM field_number WHERE field = %s AND value %s %s",
			phField, p.Op.String(), phVal)

		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("NUMBER %s%s%v", p.Field, p.Op.String(), p.Value))
		return resultName, nil

	case query.NumberRange:
		// Handle implicit created/updated fields (timestamps as number ranges)
		if p.Field == "created" || p.Field == "updated" {
			resultName := c.nextCTEName()
			col := "created_at"
			if p.Field == "updated" {
				col = "updated_at"
			}
			phLo := c.builder.Arg(int64(p.Lo))
			phHi := c.builder.Arg(int64(p.Hi))
			sql := fmt.Sprintf("SELECT id AS item_id FROM items WHERE %s >= %s AND %s <= %s", col, phLo, col, phHi)
			c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
			c.explainSteps = append(c.explainSteps, fmt.Sprintf("IMPLICIT TIMESTAMP RANGE %s:%v..%v", p.Field, p.Lo, p.Hi))
			return resultName, nil
		}

		spec, ok := c.schema.Get(p.Field)
		if !ok {
			return "", fmt.Errorf("unknown field: %s", p.Field)
		}
		if spec.Type != storage.FieldType("number") {
			return "", fmt.Errorf("field %s is not a number field", p.Field)
		}

		resultName := c.nextCTEName()
		phField := c.builder.Arg(p.Field)
		phLo := c.builder.Arg(p.Lo)
		phHi := c.builder.Arg(p.Hi)
		sql := fmt.Sprintf("SELECT item_id FROM field_number WHERE field = %s AND value >= %s AND value <= %s",
			phField, phLo, phHi)

		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("NUMBER %s:%v..%v", p.Field, p.Lo, p.Hi))
		return resultName, nil

	case query.DateCmpAbs:
		return c.compileDateCmpAbs(p)

	case query.DateRangeAbs:
		return c.compileDateRangeAbs(p)

	case query.DateCmpRel:
		return c.compileDateCmpRel(p)

	case query.Bool:
		spec, ok := c.schema.Get(p.Field)
		if !ok {
			return "", fmt.Errorf("unknown field: %s", p.Field)
		}
		if spec.Type != storage.FieldType("bool") {
			return "", fmt.Errorf("field %s is not a bool field", p.Field)
		}

		resultName := c.nextCTEName()
		phField := c.builder.Arg(p.Field)
		intVal := 0
		if p.Value {
			intVal = 1
		}
		phVal := c.builder.Arg(int64(intVal))
		sql := fmt.Sprintf("SELECT item_id FROM field_bool WHERE field = %s AND value = %s", phField, phVal)

		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("BOOL %s:%v", p.Field, p.Value))
		return resultName, nil

	default:
		return "", fmt.Errorf("unknown predicate type: %T", pred)
	}
}

func (c *Compiler) compileKeyword(p query.Keyword) (string, error) {
	// Handle implicit created/updated fields
	if p.Field == "created" || p.Field == "updated" {
		if p.Kind != query.KeywordExact {
			return "", fmt.Errorf("wildcards not supported for implicit date fields")
		}
		epochMS, err := parseDateToEpochMS(p.Pattern)
		if err != nil {
			return "", err
		}

		resultName := c.nextCTEName()
		col := "created_at"
		if p.Field == "updated" {
			col = "updated_at"
		}
		ph := c.builder.Arg(epochMS)
		sql := fmt.Sprintf("SELECT id AS item_id FROM items WHERE %s = %s", col, ph)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("IMPLICIT DATE %s:%s", p.Field, p.Pattern))
		return resultName, nil
	}

	spec, ok := c.schema.Get(p.Field)
	if !ok {
		return "", fmt.Errorf("unknown field: %s", p.Field)
	}

	// If schema says this is a TEXT field, treat field:term as FTS query
	if spec.Type == storage.FieldType("text") {
		return c.compileText(query.Text{Field: &p.Field, FTS: p.Pattern})
	}

	// Bool fields: accept true/false via field:...
	if spec.Type == storage.FieldType("bool") && (p.Pattern == "true" || p.Pattern == "false") {
		return c.compilePredicate(query.Bool{Field: p.Field, Value: p.Pattern == "true"})
	}

	// Date fields: support equality via field:YYYY-MM-DD (exact only)
	if spec.Type == storage.FieldType("date") {
		if p.Kind != query.KeywordExact {
			return "", fmt.Errorf("wildcards not supported for date fields; use comparisons")
		}
		epochMS, err := parseDateToEpochMS(p.Pattern)
		if err != nil {
			return "", err
		}
		return c.compileDateCmpAbs(query.DateCmpAbs{Field: p.Field, Op: query.CmpEq, EpochMS: epochMS})
	}

	if spec.Type != storage.FieldType("keyword") {
		return "", fmt.Errorf("field %s type %s cannot be used with keyword predicate", p.Field, spec.Type)
	}

	resultName := c.nextCTEName()
	var sql string

	phField := c.builder.Arg(p.Field)

	switch p.Kind {
	case query.KeywordExact:
		phVal := c.builder.Arg(p.Pattern)
		sql = fmt.Sprintf("SELECT p.item_id FROM kw_dict d JOIN kw_postings p ON p.value_id = d.id WHERE d.field = %s AND d.value = %s", phField, phVal)

	case query.KeywordPrefix:
		prefix := p.Pattern[:len(p.Pattern)-1] // remove trailing *
		phVal := c.builder.Arg(prefix + "%")
		sql = fmt.Sprintf("SELECT p.item_id FROM kw_dict d JOIN kw_postings p ON p.value_id = d.id WHERE d.field = %s AND d.value LIKE %s", phField, phVal)

	case query.KeywordContains:
		inner := p.Pattern[1 : len(p.Pattern)-1] // remove leading and trailing *
		phVal := c.builder.Arg("%" + inner + "%")
		sql = fmt.Sprintf("SELECT p.item_id FROM kw_dict d JOIN kw_postings p ON p.value_id = d.id WHERE d.field = %s AND d.value LIKE %s", phField, phVal)

	case query.KeywordGlob:
		phVal := c.builder.Arg(p.Pattern)
		sql = fmt.Sprintf("SELECT p.item_id FROM kw_dict d JOIN kw_postings p ON p.value_id = d.id WHERE d.field = %s AND d.value GLOB %s", phField, phVal)
	}

	c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
	c.explainSteps = append(c.explainSteps, fmt.Sprintf("KEYWORD %s:%s", p.Field, p.Pattern))
	return resultName, nil
}

func (c *Compiler) compileText(p query.Text) (string, error) {
	c.requiresFTSJoin = true
	c.textPreds = append(c.textPreds, TextPredicate{Field: p.Field, Query: p.FTS})

	resultName := c.nextCTEName()

	// Build MATCH string
	quotedFTS := quoteFTSTerm(p.FTS)
	var matchStr string

	if p.Field != nil {
		// Validate text field exists
		spec, ok := c.schema.Get(*p.Field)
		if !ok {
			return "", fmt.Errorf("unknown field: %s", *p.Field)
		}
		if spec.Type != storage.FieldType("text") {
			return "", fmt.Errorf("FTS predicate used on non-text field %s", *p.Field)
		}
		matchStr = fmt.Sprintf("%s:%s", *p.Field, quotedFTS)
	} else {
		// Bare text - search all text fields
		cols := c.schema.TextFieldsInOrder()
		if len(cols) == 0 {
			return "", fmt.Errorf("no text fields in schema for bare text query")
		}
		var parts []string
		for _, tf := range cols {
			parts = append(parts, fmt.Sprintf("%s:%s", tf.Name, quotedFTS))
		}
		matchStr = fmt.Sprintf("(%s)", joinOr(parts))
	}

	phMatch := c.builder.Arg(matchStr)
	sql := fmt.Sprintf("SELECT rowid AS item_id FROM search WHERE search MATCH %s", phMatch)

	c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
	c.explainSteps = append(c.explainSteps, fmt.Sprintf("FTS %s", p.FTS))
	return resultName, nil
}

func (c *Compiler) compileDateCmpAbs(p query.DateCmpAbs) (string, error) {
	// Implicit created/updated => items table columns
	if p.Field == "created" || p.Field == "updated" {
		resultName := c.nextCTEName()
		col := "created_at"
		if p.Field == "updated" {
			col = "updated_at"
		}
		ph := c.builder.Arg(p.EpochMS)
		sql := fmt.Sprintf("SELECT id AS item_id FROM items WHERE %s %s %s", col, p.Op.String(), ph)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("DATE %s%s%d", p.Field, p.Op.String(), p.EpochMS))
		return resultName, nil
	}

	spec, ok := c.schema.Get(p.Field)
	if !ok {
		return "", fmt.Errorf("unknown field: %s", p.Field)
	}
	if spec.Type != storage.FieldType("date") {
		return "", fmt.Errorf("field %s is not a date field", p.Field)
	}

	resultName := c.nextCTEName()
	phField := c.builder.Arg(p.Field)
	phVal := c.builder.Arg(p.EpochMS)
	sql := fmt.Sprintf("SELECT item_id FROM field_date WHERE field = %s AND value %s %s", phField, p.Op.String(), phVal)

	c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
	c.explainSteps = append(c.explainSteps, fmt.Sprintf("DATE %s%s%d", p.Field, p.Op.String(), p.EpochMS))
	return resultName, nil
}

func (c *Compiler) compileDateRangeAbs(p query.DateRangeAbs) (string, error) {
	// Implicit created/updated ranges compile to items table
	if p.Field == "created" || p.Field == "updated" {
		resultName := c.nextCTEName()
		col := "created_at"
		if p.Field == "updated" {
			col = "updated_at"
		}
		phLo := c.builder.Arg(p.LoMS)
		phHi := c.builder.Arg(p.HiMS)
		sql := fmt.Sprintf("SELECT id AS item_id FROM items WHERE %s >= %s AND %s <= %s", col, phLo, col, phHi)
		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("IMPLICIT DATE RANGE %s:%d..%d", p.Field, p.LoMS, p.HiMS))
		return resultName, nil
	}

	spec, ok := c.schema.Get(p.Field)
	if !ok {
		return "", fmt.Errorf("unknown field: %s", p.Field)
	}
	if spec.Type != storage.FieldType("date") {
		return "", fmt.Errorf("field %s is not a date field", p.Field)
	}

	resultName := c.nextCTEName()
	phField := c.builder.Arg(p.Field)
	phLo := c.builder.Arg(p.LoMS)
	phHi := c.builder.Arg(p.HiMS)
	sql := fmt.Sprintf("SELECT item_id FROM field_date WHERE field = %s AND value >= %s AND value <= %s", phField, phLo, phHi)

	c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
	c.explainSteps = append(c.explainSteps, fmt.Sprintf("DATE %s:%d..%d", p.Field, p.LoMS, p.HiMS))
	return resultName, nil
}

func (c *Compiler) compileDateCmpRel(p query.DateCmpRel) (string, error) {
	durationMS := p.Unit.ToMillis(p.Amount)

	// Relative semantics:
	// - created/updated: interpret as "age"
	// - schema date fields: interpret as "offset from now"
	isImplicit := p.Field == "created" || p.Field == "updated"

	var targetMS int64
	if isImplicit {
		targetMS = c.nowMS - durationMS
	} else {
		targetMS = c.nowMS + durationMS
	}

	if isImplicit {
		resultName := c.nextCTEName()
		col := "created_at"
		if p.Field == "updated" {
			col = "updated_at"
		}
		// Age semantics mapping:
		//   <Nd => >= now-Nd
		//   >Nd => <= now-Nd
		var mappedOp query.CmpOp
		switch p.Op {
		case query.CmpLt, query.CmpLte:
			mappedOp = query.CmpGte
		case query.CmpGt, query.CmpGte:
			mappedOp = query.CmpLte
		case query.CmpEq:
			mappedOp = query.CmpEq
		}
		ph := c.builder.Arg(targetMS)
		sql := fmt.Sprintf("SELECT id AS item_id FROM items WHERE %s %s %s", col, mappedOp.String(), ph)

		c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
		c.explainSteps = append(c.explainSteps, fmt.Sprintf("DATE(rel-age) %s%s%d%s", p.Field, mappedOp.String(), p.Amount, p.Unit.String()))
		return resultName, nil
	}

	// For schema-defined date fields
	spec, ok := c.schema.Get(p.Field)
	if !ok {
		return "", fmt.Errorf("unknown field: %s", p.Field)
	}
	if spec.Type != storage.FieldType("date") {
		return "", fmt.Errorf("field %s is not a date field", p.Field)
	}

	resultName := c.nextCTEName()
	phField := c.builder.Arg(p.Field)
	phVal := c.builder.Arg(targetMS)
	sql := fmt.Sprintf("SELECT item_id FROM field_date WHERE field = %s AND value %s %s", phField, p.Op.String(), phVal)

	c.ctes = append(c.ctes, CTE{Name: resultName, SQL: sql})
	c.explainSteps = append(c.explainSteps, fmt.Sprintf("DATE(rel) %s%s%d%s", p.Field, p.Op.String(), p.Amount, p.Unit.String()))
	return resultName, nil
}
