// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/juju/core/logger"
)

// noopLogger is a loggo.Logger that does nothing.
type noopLogger struct {
}

// Noop is a logger.Logger that doesn't do anything.
func Noop() logger.Logger {
	return noopLogger{}
}

// Criticalf logs a message at the critical level.
func (c noopLogger) Criticalf(ctx context.Context, msg string, args ...any) {
}

// Errorf logs a message at the error level.
func (c noopLogger) Errorf(ctx context.Context, msg string, args ...any) {
}

// Warningf logs a message at the warning level.
func (c noopLogger) Warningf(ctx context.Context, msg string, args ...any) {
}

// Infof logs a message at the info level.
func (c noopLogger) Infof(ctx context.Context, msg string, args ...any) {
}

// Debugf logs a message at the debug level.
func (c noopLogger) Debugf(ctx context.Context, msg string, args ...any) {
}

// Tracef logs a message at the trace level.
func (c noopLogger) Tracef(ctx context.Context, msg string, args ...any) {
}

// Logf logs some information into the test error output.
// The provided arguments are assembled together into a string with
// fmt.Sprintf.
func (c noopLogger) Logf(ctx context.Context, level logger.Level, labels logger.Labels, msg string, args ...any) {
}

// IsLevelEnabled returns true if the given level is enabled for the logger.
func (c noopLogger) IsLevelEnabled(level logger.Level) bool {
	return false
}

// Helper marks the caller as a helper function and will skip it when capturing
// the callsite location.
func (c noopLogger) Helper() {
}

// Child returns a new logger with the given name.
func (c noopLogger) Child(name string, tags ...string) logger.Logger {
	return c
}

// GetChildByName returns a child logger with the given name.
func (c noopLogger) GetChildByName(name string) logger.Logger {
	return c
}
