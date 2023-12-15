// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/logfwd/syslog"
)

// LogForwardConfig provides access to the log forwarding config for a model.
type LogForwardConfig interface {
	// WatchForLogForwardConfigChanges return a NotifyWatcher waiting for the
	// log forward configuration to change.
	WatchForLogForwardConfigChanges() (watcher.NotifyWatcher, error)

	// LogForwardConfig returns the current log forward configuration.
	LogForwardConfig() (*syslog.RawConfig, bool, error)
}

type LogSinkSpec struct {
	// Name is the name of the log sink.
	Name string

	// OpenFn is a function that opens a log sink.
	OpenFn LogSinkFn
}

// LogSinkFn is a function that opens a log sink.
type LogSinkFn func(cfg *syslog.RawConfig) (*LogSink, error)

// LogSink is a single log sink, to which log records may be sent.
type LogSink struct {
	SendCloser
}
