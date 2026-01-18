//go:build !cgo_sqlite

package main

import _ "modernc.org/sqlite"

func init() {
	sqliteDriverName = "sqlite"
}
