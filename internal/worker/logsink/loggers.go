// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"gopkg.in/tomb.v2"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	internallogger "github.com/juju/juju/internal/logger"
)

type modelLogger struct {
	tomb tomb.Tomb

	logSink       corelogger.LogSink
	agentTag      names.Tag
	modelUUID     model.UUID
	loggoContext  *loggo.Context
	loggerContext corelogger.LoggerContext
}

const modelSinkWriterName = "model-sink"

// NewModelLogger returns a new model logger instance. The model logger
// installs a TaggedRedirectWriter that forwards log records to the supplied
// log sink. When the sink's WatchRefresh channel fires (for example when the
// log router switches its active backend), the writer is removed and
// re-added so that subsequent records are delivered through the new
// backend.
func NewModelLogger(logSink corelogger.LogSink, modelUUID model.UUID, agentTag names.Tag) (worker.Worker, error) {
	loggoContext := loggo.NewContext(loggo.INFO)
	w := &modelLogger{
		logSink:       logSink,
		agentTag:      agentTag,
		modelUUID:     modelUUID,
		loggoContext:  loggoContext,
		loggerContext: internallogger.WrapLoggoContext(loggoContext),
	}
	if err := w.bindWriter(); err != nil {
		return nil, errors.Trace(err)
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Log writes the given log records to the logger's storage.
func (d *modelLogger) Log(records []corelogger.LogRecord) error {
	return d.logSink.Log(records)
}

// GetLogger returns a logger with the given name and tags.
func (d *modelLogger) GetLogger(name string, tags ...string) corelogger.Logger {
	return d.loggerContext.GetLogger(name, tags...)
}

// ConfigureLoggers configures loggers according to the given string
// specification, which specifies a set of modules and their associated
// logging levels. Loggers are colon- or semicolon-separated; each
// module is specified as <modulename>=<level>.  White space outside of
// module names and levels is ignored. The root module is specified
// with the name "<root>".
//
// An example specification:
//
//	<root>=ERROR; foo.bar=WARNING
//
// Label matching can be applied to the loggers by providing a set of labels
// to the function. If a logger has a label that matches the provided labels,
// then the logger will be configured with the provided level. If the logger
// does not have a label that matches the provided labels, then the logger
// will not be configured. No labels will configure all loggers in the
// specification.
func (d *modelLogger) ConfigureLoggers(specification string) error {
	return d.loggerContext.ConfigureLoggers(specification)
}

// ResetLoggerLevels iterates through the known logging modules and sets the
// levels of all to UNSPECIFIED, except for <root> which is set to WARNING.
// If labels are provided, then only loggers that have the provided labels
// will be reset.
func (d *modelLogger) ResetLoggerLevels() {
	d.loggerContext.ResetLoggerLevels()
}

// Config returns the current configuration of the Loggers. Loggers
// with UNSPECIFIED level will not be included.
func (d *modelLogger) Config() corelogger.Config {
	return d.loggerContext.Config()
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
	refresh := d.logSink.WatchRefresh()
	for {
		select {
		case <-d.tomb.Dying():
			return tomb.ErrDying
		case <-refresh:
			// The active backend has changed; remove the old writer
			// and re-add it so that subsequent records are delivered
			// through the new backend.
			if err := d.bindWriter(); err != nil {
				return errors.Trace(err)
			}
			refresh = d.logSink.WatchRefresh()
		}
	}
}

// bindWriter adds (or replaces) the model-sink writer in the logger context,
// binding it to the current log sink.
func (d *modelLogger) bindWriter() error {
	writer := corelogger.NewTaggedRedirectWriter(
		d.logSink,
		d.agentTag.String(),
		d.modelUUID.String(),
	)

	// We don't care about the error from RemoveWriter, since it will only fail
	// if the writer doesn't exist, which is fine.
	_, _ = d.loggoContext.RemoveWriter(modelSinkWriterName)
	if err := d.loggoContext.AddWriter(modelSinkWriterName, writer); err != nil {
		return errors.Trace(err)
	}
	return nil
}
