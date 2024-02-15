// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import "io"

// Logger provides an interface for writing log records.
type Logger interface {
	// Log writes the given log records to the logger's storage.
	Log([]LogRecord) error
}

// LoggerCloser is a Logger that can be closed.
type LoggerCloser interface {
	Logger
	io.Closer
}

// ModelLogger keeps track of loggers tied to a given model.
type ModelLogger interface {
	// Closer provides a Close() method which calls Close() on
	// each of the tracked loggers.
	io.Closer

	// GetLogger returns a logger for the given model and keeps
	// track of it, returning the same one if called again.
	GetLogger(modelUUID, modelName string) (LoggerCloser, error)

	// RemoveLogger stops tracking the given's model's logger and
	// calls Close() on the logger.
	RemoveLogger(modelUUID string) error
}

// LoggerForModelFunc is a function which returns a logger for a given model.
type LoggerForModelFunc func(modelUUID, modelName string) (LoggerCloser, error)
