// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
)

type bufferedLogWriterCloser struct {
	*corelogger.BufferedLogWriter
	closer io.Closer
}

func (b *bufferedLogWriterCloser) Close() error {
	err := errors.Trace(b.BufferedLogWriter.Flush())
	_ = b.closer.Close()
	return err
}

// NewModelLogger returns a new model logger instance.
// The actual loggers returned for each model are created
// by the supplied loggerForModelFunc.
func NewModelLogger(
	loggerForModelFunc corelogger.LogWriterForModelFunc,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock,
) corelogger.ModelLogger {
	return &modelLogger{
		clock:               clock,
		loggerBufferSize:    bufferSize,
		loggerFlushInterval: flushInterval,
		loggerForModel:      loggerForModelFunc,
		modelLoggers:        make(map[string]corelogger.LogWriterCloser),
	}
}

type modelLogger struct {
	mu sync.Mutex

	clock               clock.Clock
	loggerBufferSize    int
	loggerFlushInterval time.Duration

	modelLoggers   map[string]corelogger.LogWriterCloser
	loggerForModel corelogger.LogWriterForModelFunc
}

// GetLogWriter creates a new log writer for the given model UUID.
func (d *modelLogger) GetLogWriter(modelUUID, modelName, modelOwner string) (corelogger.LogWriterCloser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.modelLoggers[modelUUID]; ok {
		return l, nil
	}

	modelPrefix := corelogger.ModelFilePrefix(modelOwner, modelName)
	l, err := d.loggerForModel(modelUUID, modelPrefix)
	if err != nil {
		return nil, errors.Annotatef(err, "getting logger for model %q (%s)", modelPrefix, modelUUID)
	}

	bufferedLogWriter := &bufferedLogWriterCloser{
		BufferedLogWriter: corelogger.NewBufferedLogWriter(
			l,
			d.loggerBufferSize,
			d.loggerFlushInterval,
			d.clock,
		),
		closer: l,
	}
	d.modelLoggers[modelUUID] = bufferedLogWriter
	return bufferedLogWriter, nil
}

// RemoveLogWriter closes then removes a log writer by model UUID.
// Returns an error if there was a problem closing the logger.
func (d *modelLogger) RemoveLogWriter(modelUUID string) error {
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

	var errs []string
	for _, m := range d.modelLoggers {
		if err := m.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Errorf("errors closing loggers: %v", strings.Join(errs, "\n"))
}
