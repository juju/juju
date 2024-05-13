// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
)

// LogSink is a worker which provides access to a log sink
// which allows log entries to be stored for specified models.
type LogSink struct {
	tomb    tomb.Tomb
	logSink logger.ModelLogger
}

// logWriter wraps a io.Writer instance and writes out
// log records to the writer.
type logWriter struct {
	io.WriteCloser
}

// Log implements logger.Log.
func (lw *logWriter) Log(records []logger.LogRecord) error {
	for _, r := range records {
		line, err := json.Marshal(&r)
		if err != nil {
			return errors.Annotatef(err, "marshalling log record")
		}
		_, err = lw.Write([]byte(fmt.Sprintf("%s\n", line)))
		if err != nil {
			return errors.Annotatef(err, "writing log record")
		}
	}
	return nil
}

// Config defines the attributes used to create a log sink worker.
type Config struct {
	Logger                logger.Logger
	Clock                 clock.Clock
	LogSinkConfig         LogSinkConfig
	LogWriterForModelFunc logger.LogWriterForModelFunc
}

// NewWorker returns a new worker which provides access to a log sink
// which allows log entries to be stored for specified models.
func NewWorker(cfg Config) (worker.Worker, error) {
	modelLogger := NewModelLogger(
		cfg.LogWriterForModelFunc,
		cfg.LogSinkConfig.LoggerBufferSize,
		cfg.LogSinkConfig.LoggerFlushInterval,
		cfg.Clock,
	)
	w := &LogSink{
		logSink: modelLogger,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w, nil
}

// Kill implements Worker.Kill()
func (ml *LogSink) Kill() {
	ml.tomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (ml *LogSink) Wait() error {
	return ml.tomb.Wait()
}
