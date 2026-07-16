//go:build !dqlite && (!cgo || (!sqlite_trace && !trace))

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

func sqliteDriverName(appOptions) string {
	return "sqlite3"
}
