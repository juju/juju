// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/api/logsender"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

const loggerName = "juju.worker.logsender"

// LogSenderAPI provides a log writer.
type LogSenderAPI interface {
	// LogWriter returns a logsender.LogWriter which can be used to send log
	// messages to the controller. The LogWriter should be closed when finished
	// with it to free up resources.
	LogWriter(ctx context.Context) (logsender.LogWriter, error)
}

// New starts a logsender worker which reads log message structs from
// a channel and sends them to the controller via the logsink API.
func New(logs LogRecordCh, logSenderAPI LogSenderAPI) worker.Worker {
	loop := func(ctx context.Context) error {
		var logWriter logsender.LogWriter
		closeLogWriter := func() {
			if logWriter != nil {
				_ = logWriter.Close()
				logWriter = nil
			}
		}
		defer closeLogWriter()

		var pending *LogRecord
		for {
			if logWriter == nil {
				var err error
				logWriter, err = openLogWriter(ctx, logSenderAPI)
				if err != nil {
					if isRetryableLogSenderError(err) {
						if err := waitRetry(ctx); err != nil {
							return nil
						}
						continue
					}
					return errors.Annotate(err, "logsender dial failed")
				}
			}

			rec := pending
			if rec == nil {
				var ok bool
				select {
				case rec, ok = <-logs:
					if !ok {
						return nil
					}
				case <-ctx.Done():
					return nil
				}
			}

			err := writeLogRecord(logWriter, rec)
			if err == nil && rec.DroppedAfter > 0 {
				err = writeDroppedLogRecord(logWriter, rec)
			}
			if err != nil {
				if isRetryableLogSenderError(err) {
					closeLogWriter()
					pending = rec
					if err := waitRetry(ctx); err != nil {
						return nil
					}
					continue
				}
				return errors.Trace(err)
			}
			pending = nil
		}
	}
	return jworker.NewSimpleWorker(loop)
}

func openLogWriter(ctx context.Context, logSenderAPI LogSenderAPI) (logsender.LogWriter, error) {
	// It has been observed that sometimes the logsender.API gets wedged
	// attempting to get the LogWriter while the agent is being torn down,
	// and the call to logSenderAPI.LogWriter() doesn't return. This stops
	// the logsender worker from shutting down, and causes the entire agent to
	// get wedged. To mitigate this, we get the LogWriter in a different
	// goroutine allowing the worker to interrupt this.
	sender := make(chan logsender.LogWriter)
	errChan := make(chan error)
	go func() {
		logWriter, err := logSenderAPI.LogWriter(ctx)
		if err != nil {
			select {
			case errChan <- err:
			case <-ctx.Done():
			}
			return
		}
		select {
		case sender <- logWriter:
		case <-ctx.Done():
			logWriter.Close()
		}
	}()
	select {
	case logWriter := <-sender:
		return logWriter, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func writeLogRecord(logWriter logsender.LogWriter, rec *LogRecord) error {
	return logWriter.WriteLog(&params.LogRecord{
		Time:     rec.Time,
		Module:   rec.Module,
		Location: rec.Location,
		Level:    rec.Level.String(),
		Message:  rec.Message,
		Labels:   rec.Labels,
	})
}

func writeDroppedLogRecord(logWriter logsender.LogWriter, rec *LogRecord) error {
	return logWriter.WriteLog(&params.LogRecord{
		Time:    rec.Time,
		Module:  loggerName,
		Level:   loggo.WARNING.String(),
		Message: fmt.Sprintf("%d log messages dropped due to lack of API connectivity", rec.DroppedAfter),
	})
}

func isRetryableLogSenderError(err error) bool {
	if err == nil {
		return false
	}
	return isLogSinkUnavailableError(err) ||
		errors.Is(err, io.EOF) ||
		strings.Contains(err.Error(), "api caller disconnected") ||
		strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "write")
}

func waitRetry(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
