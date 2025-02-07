// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"
	"io"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	corelogger "github.com/juju/juju/core/logger"
)

type modelLogger struct {
	tomb tomb.Tomb

	bufferedLogWriter *bufferedLogWriterCloser
}

// NewModelLogger returns a new model logger instance.
// The actual loggers returned for each model are created
// by the supplied loggerForModelFunc.
func NewModelLogger(
	ctx context.Context,
	modelUUID string,
	newLogWriter corelogger.LogWriterForModelFunc,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock,
) (*modelLogger, error) {
	logger, err := newLogWriter(ctx, modelUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "getting logger for model %q", modelUUID)
	}

	bufferedLogWriter := &bufferedLogWriterCloser{
		BufferedLogWriter: corelogger.NewBufferedLogWriter(
			logger,
			bufferSize,
			flushInterval,
			clock,
		),
		closer: logger,
	}

	w := &modelLogger{
		bufferedLogWriter: bufferedLogWriter,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Log writes the given log records to the logger's storage.
func (d *modelLogger) Log(records []corelogger.LogRecord) error {
	return d.bufferedLogWriter.Log(records)
}

// Kill stops the model logger.
func (d *modelLogger) Kill() {
	d.tomb.Kill(nil)
}

// Wait waits for the model logger to stop.
func (d *modelLogger) Wait() error {
	return d.tomb.Wait()
}

func (d *modelLogger) loop() error {
	<-d.tomb.Dying()
	return tomb.ErrDying
}

type bufferedLogWriterCloser struct {
	*corelogger.BufferedLogWriter
	closer io.Closer
}

func (b *bufferedLogWriterCloser) Close() error {
	err := errors.Trace(b.BufferedLogWriter.Flush())
	_ = b.closer.Close()
	return err
}
