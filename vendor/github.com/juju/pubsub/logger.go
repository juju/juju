// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package pubsub

// Logger represents methods called by this package to a logging
// system.
type Logger interface {
	Debugf(format string, values ...interface{})
	Tracef(format string, values ...interface{})
}

type noOpLogger struct{}

func (noOpLogger) Debugf(format string, values ...interface{}) {}
func (noOpLogger) Tracef(format string, values ...interface{}) {}
