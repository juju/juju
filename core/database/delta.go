// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

// Delta represents a change to be run against the database in the
// form of a DDL/DML statement and corresponding bind arguments.
type Delta struct {
	stmt string
	args []any
}

// MakeDelta is a convenience function to return a Delta value.
func MakeDelta(stmt string, args ...any) Delta {
	return Delta{stmt: stmt, args: args}
}

// Stmt returns the DDL/DML statement.
func (d Delta) Stmt() string {
	return d.stmt
}

// Args returns the bind variables that should
// accompany the Delta's statement.
func (d Delta) Args() []any {
	return d.args
}
