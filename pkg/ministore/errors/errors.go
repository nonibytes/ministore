package errors

import (
	stderrors "errors"
	"fmt"
)

type ErrorCode string

const (
	ErrSchema       ErrorCode = "schema"
	ErrQueryParse   ErrorCode = "query_parse"
	ErrQueryReject  ErrorCode = "query_rejected"
	ErrUnknownField ErrorCode = "unknown_field"
	ErrTypeMismatch ErrorCode = "type_mismatch"
	ErrCursor       ErrorCode = "cursor"
	ErrNotFound     ErrorCode = "not_found"
	ErrBackend      ErrorCode = "backend"
	ErrNotImpl      ErrorCode = "not_implemented"
)

type Error struct {
	Code  ErrorCode
	Msg   string
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Msg, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code ErrorCode, msg string) *Error { return &Error{Code: code, Msg: msg} }
func Wrap(code ErrorCode, msg string, cause error) *Error { return &Error{Code: code, Msg: msg, Cause: cause} }

var ErrNotImplemented = stderrors.New("not implemented")
