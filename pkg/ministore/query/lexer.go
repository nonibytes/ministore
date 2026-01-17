package query

import mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"

type TokKind int

const (
	TokIdent TokKind = iota
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
)

type Tok struct {
	Kind TokKind
	S    string
	N    float64
}

func Lex(input string) ([]Tok, error) {
	return nil, mserrors.Wrap(mserrors.ErrNotImpl, "lexer", mserrors.ErrNotImplemented)
}
