package query

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Token represents a lexical token
type Token struct {
	Kind  TokenKind
	Value string
	Num   float64
}

// TokenKind is the type of token
type TokenKind int

const (
	TokIdent TokenKind = iota
	TokString
	TokNumber
	TokColon
	TokAnd
	TokOr
	TokNot
	TokLParen
	TokRParen
	TokGt
	TokGte
	TokLt
	TokLte
	TokDotDot
	TokEOF
)

func (k TokenKind) String() string {
	switch k {
	case TokIdent:
		return "Ident"
	case TokString:
		return "String"
	case TokNumber:
		return "Number"
	case TokColon:
		return "Colon"
	case TokAnd:
		return "And"
	case TokOr:
		return "Or"
	case TokNot:
		return "Not"
	case TokLParen:
		return "LParen"
	case TokRParen:
		return "RParen"
	case TokGt:
		return "Gt"
	case TokGte:
		return "Gte"
	case TokLt:
		return "Lt"
	case TokLte:
		return "Lte"
	case TokDotDot:
		return "DotDot"
	case TokEOF:
		return "EOF"
	default:
		return "Unknown"
	}
}

// Lexer tokenizes a query string
type Lexer struct {
	input []rune
	pos   int
}

// NewLexer creates a new lexer for the input string
func NewLexer(input string) *Lexer {
	return &Lexer{
		input: []rune(input),
		pos:   0,
	}
}

// Lex tokenizes the entire input
func Lex(input string) ([]Token, error) {
	lexer := NewLexer(input)
	var tokens []Token

	for {
		tok, err := lexer.Next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Kind == TokEOF {
			break
		}
	}

	return tokens, nil
}

// Next returns the next token
func (l *Lexer) Next() (Token, error) {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Kind: TokEOF}, nil
	}

	ch := l.input[l.pos]

	// Single-character tokens
	switch ch {
	case ':':
		l.pos++
		return Token{Kind: TokColon}, nil
	case '(':
		l.pos++
		return Token{Kind: TokLParen}, nil
	case ')':
		l.pos++
		return Token{Kind: TokRParen}, nil
	case '&':
		l.pos++
		return Token{Kind: TokAnd}, nil
	case '|':
		l.pos++
		return Token{Kind: TokOr}, nil
	case '!':
		l.pos++
		return Token{Kind: TokNot}, nil
	}

	// Two-character tokens
	if ch == '.' && l.peek(1) == '.' {
		l.pos += 2
		return Token{Kind: TokDotDot}, nil
	}
	if ch == '>' {
		if l.peek(1) == '=' {
			l.pos += 2
			return Token{Kind: TokGte}, nil
		}
		l.pos++
		return Token{Kind: TokGt}, nil
	}
	if ch == '<' {
		if l.peek(1) == '=' {
			l.pos += 2
			return Token{Kind: TokLte}, nil
		}
		l.pos++
		return Token{Kind: TokLt}, nil
	}

	// Quoted string
	if ch == '"' {
		return l.scanString()
	}

	// Number (including negative)
	if unicode.IsDigit(ch) || (ch == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(l.input[l.pos+1])) {
		return l.scanNumber()
	}

	// Identifier or keyword
	if isIdentStart(ch) {
		return l.scanIdent()
	}

	return Token{}, fmt.Errorf("unexpected character: %c", ch)
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(l.input[l.pos]) {
		l.pos++
	}
}

func (l *Lexer) peek(offset int) rune {
	pos := l.pos + offset
	if pos < len(l.input) {
		return l.input[pos]
	}
	return 0
}

func (l *Lexer) scanString() (Token, error) {
	l.pos++ // consume opening quote
	var sb strings.Builder

	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '"' {
			l.pos++ // consume closing quote
			return Token{Kind: TokString, Value: sb.String()}, nil
		}
		if ch == '\\' && l.pos+1 < len(l.input) {
			l.pos++
			switch l.input[l.pos] {
			case '"':
				sb.WriteRune('"')
			case '\\':
				sb.WriteRune('\\')
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			default:
				sb.WriteRune(l.input[l.pos])
			}
			l.pos++
			continue
		}
		sb.WriteRune(ch)
		l.pos++
	}

	return Token{}, fmt.Errorf("unterminated string")
}

func (l *Lexer) scanNumber() (Token, error) {
	start := l.pos

	// Handle negative sign
	if l.input[l.pos] == '-' {
		l.pos++
	}

	// Integer part
	for l.pos < len(l.input) && unicode.IsDigit(l.input[l.pos]) {
		l.pos++
	}

	// Decimal part
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		// Check if it's ".." (range operator)
		if l.peek(1) == '.' {
			// Don't consume the dots
		} else {
			l.pos++ // consume .
			for l.pos < len(l.input) && unicode.IsDigit(l.input[l.pos]) {
				l.pos++
			}
		}
	}

	numStr := string(l.input[start:l.pos])
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return Token{}, fmt.Errorf("invalid number: %s", numStr)
	}

	return Token{Kind: TokNumber, Value: numStr, Num: num}, nil
}

func (l *Lexer) scanIdent() (Token, error) {
	start := l.pos

	for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
		l.pos++
	}

	value := string(l.input[start:l.pos])
	upper := strings.ToUpper(value)

	// Check for keywords
	switch upper {
	case "AND":
		return Token{Kind: TokAnd}, nil
	case "OR":
		return Token{Kind: TokOr}, nil
	case "NOT":
		return Token{Kind: TokNot}, nil
	}

	return Token{Kind: TokIdent, Value: value}, nil
}

func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == '*' || ch == '?' || ch == '/' || ch == '-'
}

func isIdentChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '*' || ch == '?' || ch == '/' || ch == '-' || ch == '.'
}
