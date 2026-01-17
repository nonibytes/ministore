package ministore

import mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"

// Re-export error types and functions for backward compatibility
type Error = mserrors.Error
type ErrorCode = mserrors.ErrorCode

const (
	ErrSchema       = mserrors.ErrSchema
	ErrQueryParse   = mserrors.ErrQueryParse
	ErrQueryReject  = mserrors.ErrQueryReject
	ErrUnknownField = mserrors.ErrUnknownField
	ErrTypeMismatch = mserrors.ErrTypeMismatch
	ErrCursor       = mserrors.ErrCursor
	ErrNotFound     = mserrors.ErrNotFound
	ErrBackend      = mserrors.ErrBackend
	ErrNotImpl      = mserrors.ErrNotImpl
)

var ErrNotImplemented = mserrors.ErrNotImplemented

func NewError(code ErrorCode, msg string) *Error          { return mserrors.NewError(code, msg) }
func Wrap(code ErrorCode, msg string, cause error) *Error { return mserrors.Wrap(code, msg, cause) }
