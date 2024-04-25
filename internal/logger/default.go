// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/logger"
)

// GetLogger returns the default logger.
// Currently this is backed with loggo.
func GetLogger(name string, tags ...string) logger.Logger {
	return WrapLoggo(loggo.GetLoggerWithTags(name, tags...))
}

// LoggerContext returns a logger factory that creates loggers.
// Currently this is backed with loggo.
func LoggerContext(level logger.Level) logger.LoggerContext {
	return WrapLoggoContext(loggo.NewContext(loggo.Level(level)))
}

// DefaultContext returns a logger factory that creates loggers.
func DefaultContext() logger.LoggerContext {
	return WrapLoggoContext(loggo.DefaultContext())
}
