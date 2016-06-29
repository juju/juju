// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/juju/logfwd/syslog"
)

// LoggingConfig is the logging config for a model (or controller).
type LoggingConfig interface {
	// LogFwdSyslog returns the syslog forwarding config.
	LogFwdSyslog() (*syslog.RawConfig, bool)
}

// LogSinkFn is a function that opens a log sink.
type LogSinkFn func(LoggingConfig) (*LogSink, error)

// LogSink is a single log sink, to which log records may be sent.
type LogSink struct {
	SendCloser

	// Name is the name of the sink.
	Name string
}
