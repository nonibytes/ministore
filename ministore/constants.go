package ministore

import "time"

const (
	DefaultMinContainsLen     = 3
	DefaultMinPrefixLen       = 2
	DefaultMaxPrefixExpansion = 20000
	DefaultCursorTTL          = time.Hour
)
