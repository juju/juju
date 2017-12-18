// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package watcher

// Logger represents methods called by this package to a logging
// system.
type Logger interface {
	Warningf(format string, values ...interface{})
	Infof(format string, values ...interface{})
	Debugf(format string, values ...interface{})
	Tracef(format string, values ...interface{})
}

type noOpLogger struct{}

func (noOpLogger) Warningf(format string, values ...interface{}) {}
func (noOpLogger) Infof(format string, values ...interface{})    {}
func (noOpLogger) Debugf(format string, values ...interface{})   {}
func (noOpLogger) Tracef(format string, values ...interface{})   {}
