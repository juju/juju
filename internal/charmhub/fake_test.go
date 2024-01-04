// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"

	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"
)

// We can't use the juju/testing package because of circular import
// dependency issues.

// CheckLog is an interface that can be used to log messages to a
// *testing.T or *check.C.
type CheckLog interface {
	Logf(string, ...any)
}

// CheckLogger is a loggo.Logger that logs to a *testing.T or *check.C.
type CheckLogger struct {
	Log CheckLog
}

// NewCheckLogger returns a CheckLogger that logs to the given CheckLog.
func NewCheckLogger(log CheckLog) CheckLogger {
	return CheckLogger{Log: log}
}

func (c CheckLogger) Criticalf(msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("CRITICAL: %s", msg), args...)
}
func (c CheckLogger) Errorf(msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("ERROR: %s", msg), args...)
}
func (c CheckLogger) Warningf(msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("WARNING: %s", msg), args...)
}
func (c CheckLogger) Infof(msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("INFO: %s", msg), args...)
}
func (c CheckLogger) Debugf(msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("DEBUG: %s", msg), args...)
}
func (c CheckLogger) Tracef(msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("TRACE: %s", msg), args...)
}
func (c CheckLogger) Logf(level loggo.Level, msg string, args ...any) {
	c.Log.Logf(fmt.Sprintf("%s: %s", level.String(), msg), args...)
}

func (c CheckLogger) IsErrorEnabled() bool            { return true }
func (c CheckLogger) IsWarningEnabled() bool          { return true }
func (c CheckLogger) IsInfoEnabled() bool             { return true }
func (c CheckLogger) IsDebugEnabled() bool            { return true }
func (c CheckLogger) IsTraceEnabled() bool            { return true }
func (c CheckLogger) IsLevelEnabled(loggo.Level) bool { return true }

// CheckLoggerFactory is a factory for creating CheckLoggers.
type CheckLoggerFactory struct {
	c *gc.C
}

// NewCheckLoggerFactory returns a CheckLoggerFactory that creates
// CheckLoggers that log to the given *check.C.
func NewCheckLoggerFactory(c *gc.C) *CheckLoggerFactory {
	return &CheckLoggerFactory{
		c: c,
	}
}

func (c *CheckLoggerFactory) Child(string) Logger {
	return NewCheckLogger(c.c)
}
func (c *CheckLoggerFactory) ChildWithLabels(string, ...string) Logger {
	return NewCheckLogger(c.c)
}
func (c *CheckLoggerFactory) Logger() CheckLogger {
	return NewCheckLogger(c.c)
}
