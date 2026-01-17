package ministore

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrIO            ErrorKind = "io"
	ErrSQL           ErrorKind = "sql"
	ErrSchema        ErrorKind = "schema"
	ErrQueryParse    ErrorKind = "query_parse"
	ErrQueryRejected ErrorKind = "query_rejected"
	ErrUnknownField  ErrorKind = "unknown_field"
	ErrTypeMismatch  ErrorKind = "type_mismatch"
	ErrCursor        ErrorKind = "cursor"
	ErrNotFound      ErrorKind = "not_found"
	ErrFeature       ErrorKind = "feature_missing"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Field   string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	base := fmt.Sprintf("%s: %s", e.Kind, e.Message)
	if e.Field != "" {
		base = fmt.Sprintf("%s (field=%s)", base, e.Field)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", base, e.Cause)
	}
	return base
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func Wrap(kind ErrorKind, msg string, cause error) *Error {
	return &Error{Kind: kind, Message: msg, Cause: cause}
}

func New(kind ErrorKind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}

func SchemaError(msg string) *Error {
	return &Error{Kind: ErrSchema, Message: msg}
}

func QueryParseError(msg string) *Error {
	return &Error{Kind: ErrQueryParse, Message: msg}
}

func QueryRejectedError(msg string) *Error {
	return &Error{Kind: ErrQueryRejected, Message: msg}
}

func UnknownFieldError(field string) *Error {
	return &Error{Kind: ErrUnknownField, Message: "unknown field", Field: field}
}

func TypeMismatch(field, msg string) *Error {
	return &Error{Kind: ErrTypeMismatch, Field: field, Message: msg}
}

func CursorError(msg string) *Error {
	return &Error{Kind: ErrCursor, Message: msg}
}

func NotFoundError(path string) *Error {
	return &Error{Kind: ErrNotFound, Message: fmt.Sprintf("item not found: %s", path)}
}

func IsKind(err error, kind ErrorKind) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind == kind
	}
	return false
}
