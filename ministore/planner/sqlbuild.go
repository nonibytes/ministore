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
	schema storage.Schema,
	compiled *CompileOutput,
	rank RankMode,
	limitPlusOne int,
	afterFilter string,
	builder storage.Builder,
) (string, error) {
	var cteParts []string

	// Build CTEs
	for _, cte := range compiled.CTEs {
		cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", cte.Name, cte.SQL))
	}

	// Field ranking CTE if needed
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

	var withClause string
	if len(cteParts) > 0 {
		withClause = fmt.Sprintf("WITH %s ", strings.Join(cteParts, ", "))
	}

	// Build main SELECT with proper ranking
	selectColsInner := "i.id AS item_id, i.path AS path, i.data_json AS data_json, i.created_at AS created_at, i.updated_at AS updated_at"

	var orderClause, scoreExpr, ftsJoin, extraJoin string

	if rank.Kind == RankDefault && compiled.RequiresFTSJoin {
		// Weighted BM25: score = -bm25(search, w1, w2, ...)
		textFields := schema.TextFieldsInOrder()
		var weights []string
		for _, tf := range textFields {
			weights = append(weights, fmt.Sprintf("%g", tf.Weight))
		}
		weightsStr := strings.Join(weights, ", ")

		scoreExpr = fmt.Sprintf("(-bm25(search, %s))", weightsStr)
		orderClause = "ORDER BY score DESC, item_id ASC"
		ftsJoin = "JOIN search ON search.rowid = i.id"
	} else {
		switch rank.Kind {
		case RankRecency:
			orderClause = "ORDER BY updated_at DESC, path ASC"
			scoreExpr = "CAST(i.updated_at AS REAL)"
		case RankField:
			orderClause = "ORDER BY score DESC, updated_at DESC, path ASC"
			scoreExpr = fmt.Sprintf("CAST(%s.rank_value AS REAL)", fieldRankCTEName)
			extraJoin = fmt.Sprintf("JOIN %s ON %s.item_id = i.id", fieldRankCTEName, fieldRankCTEName)
		case RankNone:
			orderClause = "ORDER BY item_id ASC"
			scoreExpr = "NULL"
		case RankDefault:
			// Default without FTS - fallback to recency
			orderClause = "ORDER BY updated_at DESC, path ASC"
			scoreExpr = "CAST(i.updated_at AS REAL)"
		}
	}

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
  %s
  JOIN %s r ON r.item_id = i.id
) q
WHERE 1=1 %s
%s
LIMIT %d`,
		withClause,
		selectColsInner,
		scoreExpr,
		ftsJoin,
		extraJoin,
		compiled.ResultCTE,
		afterWhere,
		orderClause,
		limitPlusOne,
	)

	return sql, nil
}

// BuildAfterFilter builds the after-filter fragment for cursor pagination
func BuildAfterFilter(rank RankMode, hasFTS bool, builder storage.Builder, score float64, itemID int64, updatedAtMS int64, path string) (string, error) {
	switch rank.Kind {
	case RankNone:
		ph := builder.Arg(itemID)
		return fmt.Sprintf("item_id > %s", ph), nil

	case RankDefault:
		if hasFTS {
			// Default w/ FTS: ORDER BY score DESC, item_id ASC
			phScore1 := builder.Arg(score)
			phScore2 := builder.Arg(score)
			phItemID := builder.Arg(itemID)
			return fmt.Sprintf("(score < %s OR (score = %s AND item_id > %s))", phScore1, phScore2, phItemID), nil
		}
		// Fallback recency
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
