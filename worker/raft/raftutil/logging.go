// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftutil

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/juju/loggo"
)

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
	Prefix string
}

// Write is part of the io.Writer interface.
func (w *LoggoWriter) Write(p []byte) (int, error) {
	w.Logger.Logf(w.Level, "%s%s", w.Prefix, p[:len(p)-1]) // omit trailing newline
	return len(p), nil
}

func NewHCLLogger(name string, logger Logger) hclog.Logger {
	logWriter := &LoggoWriter{
		Logger: logger,
		Level:  loggo.DEBUG,
		Prefix: fmt.Sprintf("[%s] ", name),
	}

	return hclog.New(&hclog.LoggerOptions{
		Name:   name,
		Output: logWriter,
		Level:  hclog.DefaultLevel,
	})
}
