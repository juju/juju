// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"io"
)

// Logger exposes the logger functionality needed by CloseAndLog.
type Logger interface {
	// Errorf formats the provided log message and writes it to the log.
	Errorf(string, ...interface{})
}

// CloseAndLog calls the closer's Close() and logs any error returned therefrom.
func CloseAndLog(closer io.Closer, label string, logger Logger) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		logger.Errorf("while closing %s: %v", label, err)
	}
}
