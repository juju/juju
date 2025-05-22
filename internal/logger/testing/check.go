// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/logger"
)

// CheckLogger is an interface that can be used to log messages to a
// *testing.T or *check.C.
type CheckLogger interface {
	Logf(string, ...any)
	Context() context.Context
	Helper()
}

// helper exposes Helper method without introducing a new callsite.
type helper interface {
	Helper()
}

// checkLogger is a loggo.Logger that logs to a *testing.T or *check.C.
type checkLogger struct {
	helper
	log   CheckLogger
	level logger.Level
	name  string
}

// WrapCheckLog returns a checkLogger that logs to the given CheckLog.
func WrapCheckLog(log CheckLogger) logger.Logger {
	return WrapCheckLogWithLevel(log, logger.TRACE)
}

// WrapCheckLogWithLevel returns a checkLogger that logs to the given CheckLog
// with the given default level.
func WrapCheckLogWithLevel(log CheckLogger, level logger.Level) logger.Logger {
	return checkLogger{
		helper: log,
		log:    log,
		level:  level,
	}
}

func formatMsg(level, name, msg string) string {
	if name == "" {
		return fmt.Sprintf("%s: ", level) + msg
	}
	return fmt.Sprintf("%s: %s ", level, name) + msg
}

func (c checkLogger) Criticalf(ctx context.Context, msg string, args ...any) {
	c.log.Helper()
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg("CRITICAL", c.name, msg), args...)
}

func (c checkLogger) Errorf(ctx context.Context, msg string, args ...any) {
	c.log.Helper()
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg("ERROR", c.name, msg), args...)
}

func (c checkLogger) Warningf(ctx context.Context, msg string, args ...any) {
	c.log.Helper()
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg("WARNING", c.name, msg), args...)
}

func (c checkLogger) Infof(ctx context.Context, msg string, args ...any) {
	c.log.Helper()
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg("INFO", c.name, msg), args...)
}

func (c checkLogger) Debugf(ctx context.Context, msg string, args ...any) {
	c.log.Helper()
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg("DEBUG", c.name, msg), args...)
}

func (c checkLogger) Tracef(ctx context.Context, msg string, args ...any) {
	c.log.Helper()
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg("TRACE", c.name, msg), args...)
}

func (c checkLogger) Logf(ctx context.Context, level logger.Level, labels logger.Labels, msg string, args ...any) {
	c.log.Helper()
	if !c.IsLevelEnabled(level) {
		return
	}
	select {
	case <-c.log.Context().Done():
		return
	default:
	}
	c.log.Logf(formatMsg(loggo.Level(level).String(), c.name, msg), args...)
}

func (c checkLogger) Child(name string, tags ...string) logger.Logger {
	return checkLogger{helper: c.log, log: c.log, name: name}
}

// GetChildByName returns a child logger with the given name.
func (c checkLogger) GetChildByName(name string) logger.Logger {
	return checkLogger{helper: c.log, log: c.log, name: name}
}

func (c checkLogger) IsLevelEnabled(level logger.Level) bool {
	return level >= c.level
}

// WrapCheckLogForContext returns a logger.LoggerContext that creates loggers
// that log to the given CheckLogger.
func WrapCheckLogForContext(log CheckLogger) logger.LoggerContext {
	return checkLoggerContext{
		logger: log,
	}
}

type checkLoggerContext struct {
	logger CheckLogger
}

// GetLogger returns a logger with the given name and tags.
func (c checkLoggerContext) GetLogger(name string, tags ...string) logger.Logger {
	return WrapCheckLog(c.logger)
}

// ResetLoggerLevels iterates through the known logging modules and sets the
// levels of all to UNSPECIFIED, except for <root> which is set to WARNING.
// If labels are provided, then only loggers that have the provided labels
// will be reset.
func (c checkLoggerContext) ResetLoggerLevels() {}

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
func (c checkLoggerContext) ConfigureLoggers(specification string) error {
	return errors.NotImplementedf("ConfigureLoggers")
}

// Config returns the current configuration of the Loggers. Loggers
// with UNSPECIFIED level will not be included.
func (c checkLoggerContext) Config() logger.Config {
	return make(logger.Config)
}

// AddWriter adds a writer to the list to be called for each logging call.
// The name cannot be empty, and the writer cannot be nil. If an existing
// writer exists with the specified name, an error is returned.
//
// Note: we're relying on loggo.Writer here, until we do model level logging.
// Deprecated: This will be removed in the future and is only here whilst
// we cut things across.
func (c checkLoggerContext) AddWriter(name string, writer loggo.Writer) error {
	return errors.NotImplementedf("AddWriter")
}
