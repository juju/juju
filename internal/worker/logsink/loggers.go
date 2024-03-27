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

type bufferedLoggerCloser struct {
	*corelogger.BufferedLogger
	closer io.Closer
}

func (b *bufferedLoggerCloser) Close() error {
	err := errors.Trace(b.BufferedLogger.Flush())
	_ = b.closer.Close()
	return err
}

// NewModelLogger returns a new model logger instance.
// The actual loggers returned for each model are created
// by the supplied loggerForModelFunc.
func NewModelLogger(
	loggerForModelFunc corelogger.LoggerForModelFunc,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock,
) corelogger.ModelLogger {
	return &modelLogger{
		clock:               clock,
		loggerBufferSize:    bufferSize,
		loggerFlushInterval: flushInterval,
		loggerForModel:      loggerForModelFunc,
		modelLoggers:        make(map[string]corelogger.LoggerCloser),
	}
}

type modelLogger struct {
	mu sync.Mutex

	clock               clock.Clock
	loggerBufferSize    int
	loggerFlushInterval time.Duration

	modelLoggers   map[string]corelogger.LoggerCloser
	loggerForModel corelogger.LoggerForModelFunc
}

// GetLogger implements ModelLogger.
func (d *modelLogger) GetLogger(modelUUID, modelName, modelOwner string) (corelogger.LoggerCloser, error) {
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

	bufferedLogger := &bufferedLoggerCloser{
		BufferedLogger: corelogger.NewBufferedLogger(
			l,
			d.loggerBufferSize,
			d.loggerFlushInterval,
			d.clock,
		),
		closer: l,
	}
	d.modelLoggers[modelUUID] = bufferedLogger
	return bufferedLogger, nil
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

	var errs []string
	for _, m := range d.modelLoggers {
		if err := m.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		d.modelLoggers = make(map[string]corelogger.LoggerCloser)
		return nil
	}
	return errors.Errorf("errors closing loggers: %v", strings.Join(errs, "\n"))
}
