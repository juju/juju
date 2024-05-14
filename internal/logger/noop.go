// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/juju/core/logger"
)

// noopLogger is a loggo.Logger that does nothing.
type noopLogger struct {
}

// Noop is a logger.Logger that doesn't do anything.
func Noop() logger.Logger {
	return noopLogger{}
}

// Critical logs a message at the critical level.
func (c noopLogger) Criticalf(msg string, args ...any) {
}

// Error logs a message at the error level.
func (c noopLogger) Errorf(msg string, args ...any) {
}

// Warning logs a message at the warning level.
func (c noopLogger) Warningf(msg string, args ...any) {
}

// Info logs a message at the info level.
func (c noopLogger) Infof(msg string, args ...any) {
}

// Debug logs a message at the debug level.
func (c noopLogger) Debugf(msg string, args ...any) {
}

// Trace logs a message at the trace level.
func (c noopLogger) Tracef(msg string, args ...any) {
}

// Log logs some information into the test error output.
// The provided arguments are assembled together into a string with
// fmt.Sprintf.
func (c noopLogger) Logf(level logger.Level, msg string, args ...any) {
}

// IsLevelEnabled returns true if the given level is enabled for the logger.
func (c noopLogger) IsLevelEnabled(level logger.Level) bool {
	return false
}

// Child returns a new logger with the given name.
func (c noopLogger) Child(name string, tags ...string) logger.Logger {
	return c
}

// GetChildByName returns a child logger with the given name.
func (c noopLogger) GetChildByName(name string) logger.Logger {
	return c
}
