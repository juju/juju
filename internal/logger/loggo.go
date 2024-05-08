// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/logger"
)

// loggoLogger is a loggo.Logger that logs to a *testing.T or *check.C.
type loggoLogger struct {
	logger loggo.Logger
}

// WrapLoggo wraps a loggo.Logger as a logger.Logger.
func WrapLoggo(logger loggo.Logger) logger.Logger {
	return loggoLogger{logger: logger}
}

// Critical logs a message at the critical level.
func (c loggoLogger) Criticalf(msg string, args ...any) {
	c.logger.Criticalf(msg, args...)
}

// Error logs a message at the error level.
func (c loggoLogger) Errorf(msg string, args ...any) {
	c.logger.Errorf(msg, args...)
}

// Warning logs a message at the warning level.
func (c loggoLogger) Warningf(msg string, args ...any) {
	c.logger.Warningf(msg, args...)
}

// Info logs a message at the info level.
func (c loggoLogger) Infof(msg string, args ...any) {
	c.logger.Infof(msg, args...)
}

// Debug logs a message at the debug level.
func (c loggoLogger) Debugf(msg string, args ...any) {
	c.logger.Debugf(msg, args...)
}

// Trace logs a message at the trace level.
func (c loggoLogger) Tracef(msg string, args ...any) {
	c.logger.Tracef(msg, args...)
}

// Log logs some information into the test error output.
// The provided arguments are assembled together into a string with
// fmt.Sprintf.
func (c loggoLogger) Logf(level logger.Level, msg string, args ...any) {
	c.logger.Logf(loggo.Level(level), msg, args...)
}

// IsEnabled returns true if the given level is enabled for the logger.
func (c loggoLogger) IsErrorEnabled() bool {
	return c.logger.IsErrorEnabled()
}

// IsWarningEnabled returns true if the warning level is enabled for the
// logger.
func (c loggoLogger) IsWarningEnabled() bool {
	return c.logger.IsWarningEnabled()
}

// IsInfoEnabled returns true if the info level is enabled for the logger.
func (c loggoLogger) IsInfoEnabled() bool {
	return c.logger.IsInfoEnabled()
}

// IsDebugEnabled returns true if the debug level is enabled for the logger.
func (c loggoLogger) IsDebugEnabled() bool {
	return c.logger.IsDebugEnabled()
}

// IsTraceEnabled returns true if the trace level is enabled for the logger.
func (c loggoLogger) IsTraceEnabled() bool {
	return c.logger.IsTraceEnabled()
}

// IsLevelEnabled returns true if the given level is enabled for the logger.
func (c loggoLogger) IsLevelEnabled(level logger.Level) bool {
	return c.logger.IsLevelEnabled(loggo.Level(level))
}

// Child returns a new logger with the given name.
func (c loggoLogger) Child(name string) logger.Logger {
	return loggoLogger{
		logger: c.logger.Child(name),
	}
}

// ChildWithTags returns a new logger with the given name and tags.
func (c loggoLogger) ChildWithTags(name string, tags ...string) logger.Logger {
	return loggoLogger{
		logger: c.logger.ChildWithTags(name, tags...),
	}
}

// GetChildByName returns a child logger with the given name.
func (c loggoLogger) GetChildByName(name string) logger.Logger {
	return loggoLogger{
		logger: c.logger.Root().Child(name),
	}
}

type loggoLoggerContext struct {
	context *loggo.Context
}

// WrapLoggoContext wraps a loggo.Context as a logger.LoggerContext.
func WrapLoggoContext(context *loggo.Context) logger.LoggerContext {
	return loggoLoggerContext{
		context: context,
	}
}

// GetLogger returns a logger with the given name and tags.
func (c loggoLoggerContext) GetLogger(name string, tags ...string) logger.Logger {
	return WrapLoggo(c.context.GetLogger(name, tags...).WithCallDepth(3))
}

// ResetLoggerLevels iterates through the known logging modules and sets the
// levels of all to UNSPECIFIED, except for <root> which is set to WARNING.
// If labels are provided, then only loggers that have the provided labels
// will be reset.
func (c loggoLoggerContext) ResetLoggerLevels() {
	c.context.ResetLoggerLevels()
}

// ConfigureLoggers configures loggers according to the given string
// specification, which specifies a set of modules and their associated
// logging levels. Loggers are colon- or semicolon-separated; each
// module is specified as <modulename>=<level>.  White space outside of
// module names and levels is ignored. The root module is specified
// with the name "<root>".
//
// An example specification:
//
//	<root>=ERROR; foo.bar=WARNING
//
// Label matching can be applied to the loggers by providing a set of labels
// to the function. If a logger has a label that matches the provided labels,
// then the logger will be configured with the provided level. If the logger
// does not have a label that matches the provided labels, then the logger
// will not be configured. No labels will configure all loggers in the
// specification.
func (c loggoLoggerContext) ConfigureLoggers(specification string) error {
	return c.context.ConfigureLoggers(specification)
}

// Config returns the current configuration of the Loggers. Loggers
// with UNSPECIFIED level will not be included.
func (c loggoLoggerContext) Config() logger.Config {
	coerced := make(logger.Config)
	for k, v := range c.context.Config() {
		coerced[k] = logger.Level(v)
	}
	return coerced
}

// AddWriter adds a writer to the list to be called for each logging call.
// The name cannot be empty, and the writer cannot be nil. If an existing
// writer exists with the specified name, an error is returned.
//
// Note: we're relying on loggo.Writer here, until we do model level logging.
// Deprecated: This will be removed in the future and is only here whilst
// we cut things across.
func (c loggoLoggerContext) AddWriter(name string, writer loggo.Writer) error {
	return c.context.AddWriter(name, writer)
}
