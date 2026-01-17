package query

import mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"

func Normalize(expr Expr) (Expr, error) {
	return Expr{}, mserrors.Wrap(mserrors.ErrNotImpl, "normalize", mserrors.ErrNotImplemented)
}
