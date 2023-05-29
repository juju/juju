// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

// SlowQueryLogger is a logger that can be used to log slow operations.
type SlowQueryLogger interface {
	// RecordSlowQuery logs the slow query, with the given arguments.
	RecordSlowQuery(msg, stmt string, args []any, duration float64)
}

// NoopSlowQueryLogger is a logger that can be substituted for a SlowQueryLogger
// when slow query logging is not desired.
type NoopSlowQueryLogger struct{}

// RecordSlowQuery logs the slow query, with the given arguments.
func (NoopSlowQueryLogger) RecordSlowQuery(string, string, []any, float64) {}
