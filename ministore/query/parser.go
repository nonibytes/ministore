package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse parses a query string into an expression AST
func Parse(input string) (Expr, error) {
	tokens, err := Lex(input)
	if err != nil {
		return nil, err
	}

	p := &parser{tokens: tokens, pos: 0}
	return p.parseExpr()
}

type parser struct {
	tokens []Token
	pos    int
}

func (p *parser) parseExpr() (Expr, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.match(TokOr) {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = Or{Left: left, Right: right}
	}

	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for p.match(TokAnd) {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = And{Left: left, Right: right}
	}

	return left, nil
}

func (p *parser) parseNot() (Expr, error) {
	if p.match(TokNot) {
		p.advance()

		// Shorthand: !archived or NOT archived => archived:false
		// Only when next token is an identifier AND not followed by ':' or comparison op
		if p.match(TokIdent) {
			fieldName := p.current().Value
			next := p.peek(1)
			isFielded := next.Kind == TokColon || next.Kind == TokGt || next.Kind == TokGte || next.Kind == TokLt || next.Kind == TokLte
			if !isFielded {
				p.advance() // consume ident
				return Pred{Predicate: Bool{Field: fieldName, Value: false}}, nil
			}
		}

		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return Not{Inner: inner}, nil
	}

	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	// Parenthesized expression
	if p.match(TokLParen) {
		p.advance()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.match(TokRParen) {
			return nil, fmt.Errorf("expected ')', got %v", p.current())
		}
		p.advance()
		return expr, nil
	}

	// Predicate
	pred, err := p.parsePredicate()
	if err != nil {
		return nil, err
	}
	return Pred{Predicate: pred}, nil
}

func (p *parser) parsePredicate() (Predicate, error) {
	// A predicate starts with either an Ident or a String (quoted)
	var first string
	switch p.current().Kind {
	case TokIdent:
		first = p.current().Value
	case TokString:
		first = p.current().Value
	case TokEOF:
		return nil, fmt.Errorf("unexpected end of query")
	default:
		return nil, fmt.Errorf("expected term, got %v", p.current())
	}
	p.advance()

	// field:value or has:value
	if p.match(TokColon) {
		p.advance()
		// Special handling for has:<field>
		if first == "has" {
			f, err := p.expectStringOrIdent()
			if err != nil {
				return nil, err
			}
			return Has{Field: f}, nil
		}
		return p.parseFieldPredicate(first)
	}

	// field comparisons: priority>5, due<2024-01-01, created<7d
	if p.match(TokGt) || p.match(TokGte) || p.match(TokLt) || p.match(TokLte) {
		return p.parseComparison(first)
	}

	if p.match(TokDotDot) {
		return nil, fmt.Errorf("range requires field:start..end notation")
	}

	// Bare term => full-text across all text fields
	return Text{Field: nil, FTS: first}, nil
}

func (p *parser) parseFieldPredicate(field string) (Predicate, error) {
	// Special handling for path field
	if field == "path" {
		pattern, err := p.expectStringOrIdent()
		if err != nil {
			return nil, err
		}
		return PathGlob{Pattern: pattern}, nil
	}

	// Get value
	switch p.current().Kind {
	case TokString, TokIdent:
		value := p.current().Value
		p.advance()

		// Support date ranges: field:2024-01-01..2024-06-30
		if p.match(TokDotDot) {
			p.advance()
			hiStr, err := p.expectStringOrIdent()
			if err != nil {
				return nil, err
			}
			loMS, err := parseDateToEpochMS(value)
			if err != nil {
				return nil, err
			}
			hiMS, err := parseDateToEpochMS(hiStr)
			if err != nil {
				return nil, err
			}
			return DateRangeAbs{Field: field, LoMS: loMS, HiMS: hiMS}, nil
		}

		// Classify as keyword pattern (planner will reinterpret based on schema type)
		kind := classifyKeywordPattern(value)
		return Keyword{Field: field, Pattern: value, Kind: kind}, nil

	case TokNumber:
		val := p.current().Num
		p.advance()

		// Check for range (..)
		if p.match(TokDotDot) {
			p.advance()
			hi, err := p.expectNumber()
			if err != nil {
				return nil, err
			}
			return NumberRange{Field: field, Lo: val, Hi: hi}, nil
		}

		// Single number as equality check
		return NumberCmp{Field: field, Op: CmpEq, Value: val}, nil

	default:
		return nil, fmt.Errorf("expected value after '%s:'", field)
	}
}

func (p *parser) parseComparison(field string) (Predicate, error) {
	var op CmpOp
	switch p.current().Kind {
	case TokGt:
		op = CmpGt
	case TokGte:
		op = CmpGte
	case TokLt:
		op = CmpLt
	case TokLte:
		op = CmpLte
	default:
		return nil, fmt.Errorf("expected comparison operator")
	}
	p.advance()

	// For numbers
	if p.match(TokNumber) {
		val := p.current().Num
		p.advance()
		return NumberCmp{Field: field, Op: op, Value: val}, nil
	}

	// For dates or relative dates
	if p.match(TokIdent) || p.match(TokString) {
		s := p.current().Value
		p.advance()

		// Check if it's a relative date (e.g., "3d", "2w")
		if amount, unit, ok := parseRelativeDuration(s); ok {
			return DateCmpRel{Field: field, Op: op, Amount: amount, Unit: unit}, nil
		}

		// Otherwise parse as absolute date/datetime
		epochMS, err := parseDateToEpochMS(s)
		if err != nil {
			return nil, err
		}
		return DateCmpAbs{Field: field, Op: op, EpochMS: epochMS}, nil
	}

	return nil, fmt.Errorf("expected value after comparison operator")
}

func (p *parser) current() Token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return Token{Kind: TokEOF}
}

func (p *parser) peek(offset int) Token {
	pos := p.pos + offset
	if pos < len(p.tokens) {
		return p.tokens[pos]
	}
	return Token{Kind: TokEOF}
}

func (p *parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *parser) match(kind TokenKind) bool {
	return p.current().Kind == kind
}

func (p *parser) expectStringOrIdent() (string, error) {
	if p.match(TokString) || p.match(TokIdent) {
		result := p.current().Value
		p.advance()
		return result, nil
	}
	return "", fmt.Errorf("expected string or identifier, got %v", p.current())
}

func (p *parser) expectNumber() (float64, error) {
	if p.match(TokNumber) {
		result := p.current().Num
		p.advance()
		return result, nil
	}
	return 0, fmt.Errorf("expected number, got %v", p.current())
}

func classifyKeywordPattern(s string) KeywordPatternKind {
	if !strings.Contains(s, "*") && !strings.Contains(s, "?") {
		return KeywordExact
	}
	if strings.Contains(s, "?") {
		return KeywordGlob
	}
	// only '*' remains
	starCount := strings.Count(s, "*")
	if strings.HasSuffix(s, "*") && !strings.HasPrefix(s, "*") && starCount == 1 {
		return KeywordPrefix
	}
	if strings.HasPrefix(s, "*") && strings.HasSuffix(s, "*") && starCount == 2 {
		return KeywordContains
	}
	return KeywordGlob
}

func parseRelativeDuration(s string) (int64, RelUnit, bool) {
	if len(s) == 0 {
		return 0, 0, false
	}

	// Find where the unit starts
	var numPart, unitPart string
	for i, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			numPart = s[:i]
			unitPart = s[i:]
			break
		}
	}

	if numPart == "" || unitPart == "" {
		return 0, 0, false
	}

	amount, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil {
		return 0, 0, false
	}

	var unit RelUnit
	switch strings.ToLower(unitPart) {
	case "h", "hour", "hours":
		unit = RelH
	case "d", "day", "days":
		unit = RelD
	case "w", "week", "weeks":
		unit = RelW
	case "m", "month", "months":
		unit = RelM
	case "y", "year", "years":
		unit = RelY
	default:
		return 0, 0, false
	}

	return amount, unit, true
}

func parseDateToEpochMS(s string) (int64, error) {
	// Try YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UnixMilli(), nil
	}
	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli(), nil
	}
	return 0, fmt.Errorf("invalid date format: %s", s)
}
