// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

// NoopSlowQueryLogger is a logger that can be substituted for a SlowQueryLogger
// when slow query logging is not desired.
type NoopSlowQueryLogger struct{}

// Log the slow query, with the given arguments.
func (NoopSlowQueryLogger) Log(msg string, duration float64, stmt string, stack []byte) error {
	return nil
}

// Close the logger.
func (NoopSlowQueryLogger) Close() error {
	return nil
}
