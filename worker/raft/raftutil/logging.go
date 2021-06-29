// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftutil

import "github.com/juju/loggo/v2"

// Logger defines the logging methods the LoggoWriter requires.
type Logger interface {
	Logf(loggo.Level, string, ...interface{})
}

// LoggoWriter is an io.Writer that will call the embedded
// logger's Log method for each Write, using the specified
// log level.
type LoggoWriter struct {
	Logger Logger
	Level  loggo.Level
}

// Write is part of the io.Writer interface.
func (w *LoggoWriter) Write(p []byte) (int, error) {
	w.Logger.Logf(w.Level, "%s", p[:len(p)-1]) // omit trailing newline
	return len(p), nil
}
