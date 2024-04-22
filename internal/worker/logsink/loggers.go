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

	"github.com/juju/juju/core/database"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/user"
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
	logger Logger,
) (corelogger.ModelLogger, error) {
	modelLogger := &modelLogger{
		clock:  clock,
		logger: logger,

		loggerBufferSize:    bufferSize,
		loggerFlushInterval: flushInterval,
		loggerForModel:      loggerForModelFunc,

		modelLoggers: make(map[string]corelogger.LoggerCloser),
	}

	// Create the fallback logger for models that have not been initialized yet.
	if err := modelLogger.initLogger(database.ControllerNS, "log", user.AdminUserName); err != nil {
		return nil, errors.Trace(err)
	}

	return modelLogger, nil
}

type modelLogger struct {
	mu sync.Mutex

	clock  clock.Clock
	logger Logger

	loggerBufferSize    int
	loggerFlushInterval time.Duration

	modelLoggers   map[string]corelogger.LoggerCloser
	loggerForModel corelogger.LoggerForModelFunc
}

// InitLogger creates a new logger for the given model, with the model name
// and owner.
func (d *modelLogger) InitLogger(modelUUID, modelName, modelOwner string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.initLogger(modelUUID, modelName, modelOwner)
}

// GetLogger returns a logger for a given model and keeps track of it.
func (d *modelLogger) GetLogger(modelUUID string) corelogger.LoggerCloser {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, ok := d.modelLoggers[modelUUID]; ok {
		return l
	}

	// The fallback logger is used if the logger for the model has not been
	// initialized yet.
	//
	// TODO (stickupkid): This should be removed once we've implemented all
	// the set status calls for the domain types.
	d.logger.Infof("using fallback logger for model %q", modelUUID)
	return d.modelLoggers[database.ControllerNS]
}

// RemoveLogger the logger, cleans up the logger and stops tracking it.
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
		return nil
	}
	return errors.Errorf("errors closing loggers: %v", strings.Join(errs, "\n"))
}

func (d *modelLogger) initLogger(modelUUID, modelName, modelOwner string) error {
	// If we've already created a logger for this model, return an error.
	if _, ok := d.modelLoggers[modelUUID]; ok {
		return errors.AlreadyExistsf("logger for model %q", modelUUID)
	}

	modelPrefix := corelogger.ModelFilePrefix(modelOwner, modelName)
	l, err := d.loggerForModel(modelUUID, modelPrefix)
	if err != nil {
		return errors.Annotatef(err, "getting logger for model %q (%s)", modelPrefix, modelUUID)
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
	return nil
}
