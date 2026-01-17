package planner

import (
	"fmt"
	"strings"

	"github.com/ministore/ministore/ministore/storage"
)

// RankMode specifies how results should be ranked
type RankMode struct {
	Kind  RankKind
	Field string // only when Kind == RankField
}

// RankKind is the type of ranking
type RankKind int

const (
	RankDefault RankKind = iota
	RankRecency
	RankField
	RankNone
)

// BuildSearchSQL builds the final search SQL
func BuildSearchSQL(
	adapter storage.Adapter,
	schema storage.Schema,
	compiled *CompileOutput,
	rank RankMode,
	limitPlusOne int,
	afterFilter string,
	builder storage.Builder,
) (string, error) {
	var cteParts []string

	// Base CTEs
	for _, cte := range compiled.CTEs {
		cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", cte.Name, cte.SQL))
	}

	// RankField: build rank aggregation CTE
	var fieldRankCTEName string
	if rank.Kind == RankField {
		spec, ok := schema.Get(rank.Field)
		if !ok {
			return "", fmt.Errorf("unknown rank field: %s", rank.Field)
		}

		var table, valueCol string
		switch spec.Type {
		case storage.FieldType("number"):
			table = "field_number"
			valueCol = "value"
		case storage.FieldType("date"):
			table = "field_date"
			valueCol = "value"
		default:
			return "", fmt.Errorf("rank field must be number or date, got %s", spec.Type)
		}

		fieldRankCTEName = "rank_field"
		phField := builder.Arg(rank.Field)
		cteSQL := fmt.Sprintf(
			"SELECT item_id, MAX(%s) AS rank_value FROM %s WHERE field = %s GROUP BY item_id",
			valueCol, table, phField,
		)
		cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", fieldRankCTEName, cteSQL))
	}

	// RankDefault+FTS: add FTS score CTEs (positive-context text predicates only)
	var ftsJoinSQL string
	var scoreExpr string
	var orderClause string

	hasFTSScore := rank.Kind == RankDefault && len(compiled.TextPreds) > 0 && adapter.FTS().HasFTS(schema)
	if hasFTSScore {
		extraCTEs, joinSQL, score, err := adapter.FTS().ScoreCTEsAndJoin(builder, schema, compiled.TextPreds)
		if err != nil {
			return "", err
		}
		for _, c := range extraCTEs {
			cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", c.Name, c.SQL))
		}
		ftsJoinSQL = joinSQL
		scoreExpr = score
		orderClause = "ORDER BY score DESC, item_id ASC"
	}

	if !hasFTSScore {
		switch rank.Kind {
		case RankRecency:
			orderClause = "ORDER BY updated_at DESC, path ASC"
			scoreExpr = "CAST(i.updated_at AS DOUBLE PRECISION)"
		case RankField:
			orderClause = "ORDER BY score DESC, updated_at DESC, path ASC"
			scoreExpr = fmt.Sprintf("CAST(%s.rank_value AS DOUBLE PRECISION)", fieldRankCTEName)
		case RankNone:
			orderClause = "ORDER BY item_id ASC"
			scoreExpr = "NULL"
		case RankDefault:
			// Default without FTS score - fallback to recency
			orderClause = "ORDER BY updated_at DESC, path ASC"
			scoreExpr = "CAST(i.updated_at AS DOUBLE PRECISION)"
		}
	}

	var withClause string
	if len(cteParts) > 0 {
		withClause = fmt.Sprintf("WITH %s ", strings.Join(cteParts, ", "))
	}

	selectColsInner := "i.id AS item_id, i.path AS path, i.data_json AS data_json, i.created_at AS created_at, i.updated_at AS updated_at"

	var joins []string
	if ftsJoinSQL != "" {
		joins = append(joins, ftsJoinSQL)
	}
	if rank.Kind == RankField {
		joins = append(joins, fmt.Sprintf("JOIN %s ON %s.item_id = i.id", fieldRankCTEName, fieldRankCTEName))
	}
	joinsSQL := strings.Join(joins, "\n  ")

	var afterWhere string
	if afterFilter != "" {
		afterWhere = fmt.Sprintf("AND (%s)", afterFilter)
	}

	sql := fmt.Sprintf(`%s
SELECT item_id, path, data_json, created_at, updated_at, score
FROM (
  SELECT %s, %s AS score
  FROM items i
  %s
  JOIN %s r ON r.item_id = i.id
) q
WHERE 1=1 %s
%s
LIMIT %d`,
		withClause,
		selectColsInner,
		scoreExpr,
		joinsSQL,
		compiled.ResultCTE,
		afterWhere,
		orderClause,
		limitPlusOne,
	)

	return sql, nil
}

// BuildAfterFilter builds the after-filter fragment for cursor pagination
func BuildAfterFilter(rank RankMode, hasFTSScore bool, builder storage.Builder, score float64, itemID int64, updatedAtMS int64, path string) (string, error) {
	switch rank.Kind {
	case RankNone:
		ph := builder.Arg(itemID)
		return fmt.Sprintf("item_id > %s", ph), nil

	case RankDefault:
		if hasFTSScore {
			// Default w/ FTS score: ORDER BY score DESC, item_id ASC
			phScore1 := builder.Arg(score)
			phScore2 := builder.Arg(score)
			phItemID := builder.Arg(itemID)
			return fmt.Sprintf("(score < %s OR (score = %s AND item_id > %s))", phScore1, phScore2, phItemID), nil
		}
		fallthrough

	case RankRecency:
		// ORDER BY updated_at DESC, path ASC
		ph1 := builder.Arg(updatedAtMS)
		ph2 := builder.Arg(updatedAtMS)
		ph3 := builder.Arg(path)
		return fmt.Sprintf("(updated_at < %s OR (updated_at = %s AND path > %s))", ph1, ph2, ph3), nil

	case RankField:
		// ORDER BY score DESC, updated_at DESC, path ASC
		phScore1 := builder.Arg(score)
		phScore2 := builder.Arg(score)
		phUpdated1 := builder.Arg(updatedAtMS)
		phUpdated2 := builder.Arg(updatedAtMS)
		phPath := builder.Arg(path)
		return fmt.Sprintf(
			"(score < %s OR (score = %s AND (updated_at < %s OR (updated_at = %s AND path > %s))))",
			phScore1, phScore2, phUpdated1, phUpdated2, phPath,
		), nil

	default:
		return "", fmt.Errorf("unknown rank kind")
	}
}
