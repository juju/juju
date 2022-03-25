// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logdb

import (
	"io"

	"github.com/juju/juju/state"
)

// TeeLogger forwards log request to each underlying logger.
type TeeLogger struct {
	loggers []Logger
}

// NewTeeLogger returns a logger that forwards log requests to each one of the
// provider loggers.
func NewTeeLogger(loggers ...Logger) *TeeLogger {
	return &TeeLogger{loggers: loggers}
}

func (t *TeeLogger) Log(records []state.LogRecord) error {
	for _, l := range t.loggers {
		if err := l.Log(records); err != nil {
			return err
		}
	}
	return nil
}

func (t *TeeLogger) Close() error {
	for _, l := range t.loggers {
		if closer, ok := l.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	return nil
}
