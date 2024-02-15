// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"io"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
)

// LoggerCloser is a Logger that can be closed.
type LoggerCloser interface {
	Logger
	io.Closer
}

type bufferedLoggerCloser struct {
	*BufferedLogger
	closer io.Closer
}

func (b *bufferedLoggerCloser) Close() error {
	err := errors.Trace(b.BufferedLogger.Flush())
	_ = b.closer.Close()
	return err
}

// ModelLogger keeps track of loggers tied to a given model.
type ModelLogger interface {
	// GetLogger returns a logger for the given model and keeps
	// track of it, returning the same one if called again.
	GetLogger(modelUUID, modelName string) LoggerCloser

	// RemoveLogger stops tracking the given's model's logger and
	// calls Close() on the logger.
	RemoveLogger(modelUUID string) error

	// Closer provides a Close() method which calls Close() on
	// each of the tracked loggers.
	io.Closer
}

// LoggerForModelFunc is a function which returns a logger for a given model.
type LoggerForModelFunc func(modelUUID, modelName string) (LoggerCloser, error)

// NewModelLogger returns a new model logger instance.
// The actual loggers returned for each model are created
// by the supplied loggerForModelFunc.
func NewModelLogger(
	loggerForModelFunc LoggerForModelFunc,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock,
) ModelLogger {
	return &modelLogger{
		clock:               clock,
		loggerBufferSize:    bufferSize,
		loggerFlushInterval: flushInterval,
		loggerForModel:      loggerForModelFunc,
	}
}

type modelLogger struct {
	mu sync.Mutex

	clock               clock.Clock
	loggerBufferSize    int
	loggerFlushInterval time.Duration

	modelLoggers   map[string]LoggerCloser
	loggerForModel LoggerForModelFunc
}

// GetLogger implements ModelLogger.
func (d *modelLogger) GetLogger(modelUUID, modelName string) LoggerCloser {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.modelLoggers[modelUUID]; ok {
		return l
	}
	if d.modelLoggers == nil {
		d.modelLoggers = make(map[string]LoggerCloser)
	}

	l, err := d.loggerForModel(modelUUID, modelName)
	if err != nil {
		panic(err)
	}

	bufferedLogger := &bufferedLoggerCloser{
		BufferedLogger: NewBufferedLogger(
			l,
			d.loggerBufferSize,
			d.loggerFlushInterval,
			d.clock,
		),
		closer: l,
	}
	d.modelLoggers[modelUUID] = bufferedLogger
	return bufferedLogger
}

// RemoveLogger implements ModelLogger.
func (d *modelLogger) RemoveLogger(modelUUID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.modelLoggers[modelUUID]; ok {
		err := l.Close()
		delete(d.modelLoggers, modelUUID)
		return err
	}
	return nil
}

// Close implements io.Close.
func (d *modelLogger) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, m := range d.modelLoggers {
		_ = m.Close()
	}
	return nil
}
