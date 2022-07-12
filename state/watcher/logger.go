// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// Logger represents methods called by this package to a logging
// system.
type Logger interface {
	Criticalf(format string, values ...interface{})
	Warningf(format string, values ...interface{})
	Infof(format string, values ...interface{})
	Debugf(format string, values ...interface{})
	Tracef(format string, values ...interface{})
	Errorf(format string, values ...interface{})
	IsTraceEnabled() bool
}

type noOpLogger struct{}

func (noOpLogger) Criticalf(format string, values ...interface{}) {}
func (noOpLogger) Warningf(format string, values ...interface{})  {}
func (noOpLogger) Infof(format string, values ...interface{})     {}
func (noOpLogger) Debugf(format string, values ...interface{})    {}
func (noOpLogger) Tracef(format string, values ...interface{})    {}
func (noOpLogger) Errorf(format string, values ...interface{})    {}
func (noOpLogger) IsTraceEnabled() bool                           { return false }
