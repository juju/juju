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
	return WrapLoggo(loggo.GetLoggerWithTags(name, tags...).WithCallDepth(3))
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

// ConfigureLoggers configures loggers on the default context according to the
// given string specification, which specifies a set of modules and their
// associated logging levels.  Loggers are colon- or semicolon-separated; each
// module is specified as <modulename>=<level>.  White space outside of module
// names and levels is ignored.  The root module is specified with the name
// "<root>".
//
// An example specification:
//
//	`<root>=ERROR; foo.bar=WARNING`
func ConfigureLoggers(config string) error {
	return loggo.ConfigureLoggers(config)
}
