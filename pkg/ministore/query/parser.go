package query

import mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"

func Parse(input string) (Expr, error) {
	return Expr{}, mserrors.Wrap(mserrors.ErrNotImpl, "parser", mserrors.ErrNotImplemented)
}
