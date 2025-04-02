// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	internallogger "github.com/juju/juju/internal/logger"
)

type modelLogger struct {
	tomb tomb.Tomb

	logSink       corelogger.LogSink
	loggerContext corelogger.LoggerContext
}

// NewModelLogger returns a new model logger instance.
func NewModelLogger(logSink corelogger.LogSink, modelUUID model.UUID) (worker.Worker, error) {
	// Create a new logger context for the model. This will use the buffered
	// log writer to write the logs to disk.
	loggerContext := loggo.NewContext(loggo.INFO)
	if err := loggerContext.AddWriter("model-sink", modelWriter{
		logSink:   logSink,
		modelUUID: modelUUID.String(),
	}); err != nil {
		return nil, errors.Annotatef(err, "adding model-sink writer")
	}

	w := &modelLogger{
		logSink:       logSink,
		loggerContext: internallogger.WrapLoggoContext(loggerContext),
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
	// Wait for the heat death of the universe.
	<-d.tomb.Dying()
	return tomb.ErrDying
}

type modelWriter struct {
	logSink   corelogger.LogSink
	modelUUID string
}

func (w modelWriter) Write(entry loggo.Entry) {
	var location string
	if entry.Filename != "" {
		location = entry.Filename + ":" + strconv.Itoa(entry.Line)
	}

	w.logSink.Log([]corelogger.LogRecord{{
		Time:      entry.Timestamp,
		Module:    entry.Module,
		Location:  location,
		Level:     corelogger.Level(entry.Level),
		Message:   entry.Message,
		Labels:    entry.Labels,
		ModelUUID: w.modelUUID,
	}})
}
