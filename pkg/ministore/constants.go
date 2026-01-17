package ministore

import "time"

const (
	MinContainsLen     = 3
	MinPrefixLen       = 2
	MaxPrefixExpansion = 20000
)

var DefaultCursorTTL = time.Hour
