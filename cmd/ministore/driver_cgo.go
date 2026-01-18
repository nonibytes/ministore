//go:build cgo_sqlite

package main

import _ "github.com/mattn/go-sqlite3"

func init() {
	sqliteDriverName = "sqlite3"
}
