// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"context"

	"github.com/juju/juju/core/logger"
)

// WrappedLogger is a logger.Logger that logs to worker.Logger interface.
type WrappedLogger struct {
	logger logger.Logger
}

// WrapLogger returns a new instance of WrappedLogger.
func WrapLogger(logger logger.Logger) *WrappedLogger {
	return &WrappedLogger{
		logger: logger,
	}
}

// Error logs a message at the error level.
func (c *WrappedLogger) Errorf(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Errorf(context.Background(), msg, args...)
}

// Info logs a message at the info level.
func (c *WrappedLogger) Infof(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Infof(context.Background(), msg, args...)
}

// Debug logs a message at the debug level.
func (c *WrappedLogger) Debugf(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Debugf(context.Background(), msg, args...)
}
