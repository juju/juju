// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/loggo"
)

// NoopLogger is a loggo.Logger that does nothing.
type NoopLogger struct{}

func (NoopLogger) Criticalf(string, ...any) {}
func (NoopLogger) Errorf(string, ...any)    {}
func (NoopLogger) Warningf(string, ...any)  {}
func (NoopLogger) Infof(string, ...any)     {}
func (NoopLogger) Debugf(string, ...any)    {}
func (NoopLogger) Tracef(string, ...any)    {}

func (NoopLogger) IsErrorEnabled() bool   { return false }
func (NoopLogger) IsWarningEnabled() bool { return false }
func (NoopLogger) IsInfoEnabled() bool    { return false }
func (NoopLogger) IsDebugEnabled() bool   { return false }
func (NoopLogger) IsTraceEnabled() bool   { return false }

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
func (c CheckLogger) Child(name string) CheckLogger { return c }

func (c CheckLogger) IsErrorEnabled() bool   { return false }
func (c CheckLogger) IsWarningEnabled() bool { return false }
func (c CheckLogger) IsInfoEnabled() bool    { return false }
func (c CheckLogger) IsDebugEnabled() bool   { return false }
func (c CheckLogger) IsTraceEnabled() bool   { return false }
