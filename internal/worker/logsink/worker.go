// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	corelogger "github.com/juju/juju/core/logger"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// LogSink is a worker which provides access to a log sink
// which allows log entries to be stored for specified models.
type LogSink struct {
	catacomb catacomb.Catacomb
	logSink  corelogger.ModelLogger
}

// logWriter wraps a io.Writer instance and writes out
// log records to the writer.
type logWriter struct {
	io.WriteCloser
}

// Log implements logger.Log.
func (lw *logWriter) Log(records []corelogger.LogRecord) error {
	for _, r := range records {
		//TODO(debug-log) - we'll move to newline delimited json
		var labelsOut []string
		for k, v := range r.Labels {
			labelsOut = append(labelsOut, fmt.Sprintf("%s:%s", k, v))
		}
		_, err := lw.Write([]byte(strings.Join([]string{
			r.Entity,
			r.Time.In(time.UTC).Format("2006-01-02 15:04:05"),
			r.Level.String(),
			r.Module,
			r.Location,
			r.Message,
			strings.Join(labelsOut, ","),
		}, " ") + "\n"))
		if err != nil {
			return errors.Annotatef(err, "writing log record")
		}
	}
	return nil
}

// Config defines the attributes used to create a log sink worker.
type Config struct {
	Logger             Logger
	Clock              clock.Clock
	LogSinkConfig      LogSinkConfig
	LoggerForModelFunc corelogger.LoggerForModelFunc
}

// NewWorker returns a new worker which provides access to a log sink
// which allows log entries to be stored for specified models.
func NewWorker(cfg Config) (worker.Worker, error) {
	modelLogger := corelogger.NewModelLogger(
		cfg.LoggerForModelFunc,
		cfg.LogSinkConfig.LoggerBufferSize,
		cfg.LogSinkConfig.LoggerFlushInterval,
		cfg.Clock,
	)
	w := &LogSink{
		logSink: modelLogger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			<-w.catacomb.Dying()
			return nil
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill implements Worker.Kill()
func (ml *LogSink) Kill() {
	ml.catacomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (ml *LogSink) Wait() error {
	return ml.catacomb.Wait()
}
