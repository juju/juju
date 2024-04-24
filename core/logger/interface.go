// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/juju/names/v5"
)

// LogWriter provides an interface for writing log records.
type LogWriter interface {
	// Log writes the given log records to the logger's storage.
	Log([]LogRecord) error
}

// LogWriterCloser is a Logger that can be closed.
type LogWriterCloser interface {
	LogWriter
	io.Closer
}

// ModelLogger keeps track of all the log writers, which can be accessed
// by a given model uuid.
type ModelLogger interface {
	// Closer provides a Close() method which calls Close() on
	// each of the tracked log writers.
	io.Closer

	// GetLogWriter returns a log writer for the given model and keeps
	// track of it, returning the same one if called again.
	GetLogWriter(modelUUID, modelName, modelOwner string) (LogWriterCloser, error)

	// RemoveLogWriter stops tracking the given's model's log writer and
	// calls Close() on the log writer.
	RemoveLogWriter(modelUUID string) error
}

// LogWriterForModelFunc is a function which returns a log writer for a given model.
type LogWriterForModelFunc func(modelUUID, modelName string) (LogWriterCloser, error)

// ModelFilePrefix makes a log file prefix from the model owner and name.
func ModelFilePrefix(owner, name string) string {
	return fmt.Sprintf("%s-%s", owner, name)
}

// ModelLogFile makes an absolute model log file path.
func ModelLogFile(logDir, modelUUID, modelOwnerAndName string) string {
	filename := modelOwnerAndName + "-" + names.NewModelTag(modelUUID).ShortId() + ".log"
	return filepath.Join(logDir, "models", filename)
}
