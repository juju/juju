// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api/logsender"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

const loggerName = "juju.worker.logsender"

// LogSenderAPI provides a log writer.
type LogSenderAPI interface {
	LogWriter(ctx context.Context) (logsender.LogWriter, error)
}

// New starts a logsender worker which reads log message structs from
// a channel and sends them to the controller via the logsink API.
func New(logs LogRecordCh, logSenderAPI LogSenderAPI) worker.Worker {
	loop := func(ctx context.Context) error {
		// It has been observed that sometimes the logsender.API gets wedged
		// attempting to get the LogWriter while the agent is being torn down,
		// and the call to logSenderAPI.LogWriter() doesn't return. This stops
		// the logsender worker from shutting down, and causes the entire
		// agent to get wedged. To mitigate this, we get the LogWriter in a
		// different goroutine allowing the worker to interrupt this.
		sender := make(chan logsender.LogWriter)
		errChan := make(chan error)
		go func() {
			logWriter, err := logSenderAPI.LogWriter(context.TODO())
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
				// Loop has been stopped before returning the writer. Need to close it in order to avoid
				// possible ressources leak
				logWriter.Close()
			}
		}()
		var logWriter logsender.LogWriter
		var err error
		select {
		case logWriter = <-sender:
		case err = <-errChan:
			return errors.Annotate(err, "logsender dial failed")
		case <-ctx.Done():
			return nil
		}
		// the logwriter has been successfully retrieved from the inside goroutine and its lifecycle is now handled
		// in the loop function.
		defer logWriter.Close()
		for {
			select {
			case rec := <-logs:
				err := logWriter.WriteLog(&params.LogRecord{
					Time:     rec.Time,
					Module:   rec.Module,
					Location: rec.Location,
					Level:    rec.Level.String(),
					Message:  rec.Message,
					Labels:   rec.Labels,
				})
				if err != nil {
					return errors.Trace(err)
				}
				if rec.DroppedAfter > 0 {
					// If messages were dropped after this one, report
					// the count (the source of the log messages -
					// BufferedLogWriter - handles the actual dropping
					// and counting).
					//
					// Any logs indicated as dropped here are will
					// never end up in the logs DB in the JES
					// (although will still be in the local agent log
					// file). Message dropping by the
					// BufferedLogWriter is last resort protection
					// against memory exhaustion and should only
					// happen if API connectivity is lost for extended
					// periods. The maximum in-memory log buffer is
					// quite large (see the InstallBufferedLogWriter
					// call in jujuDMain).
					err := logWriter.WriteLog(&params.LogRecord{
						Time:    rec.Time,
						Module:  loggerName,
						Level:   loggo.WARNING.String(),
						Message: fmt.Sprintf("%d log messages dropped due to lack of API connectivity", rec.DroppedAfter),
					})
					if err != nil {
						return errors.Trace(err)
					}
				}

			case <-ctx.Done():
				return nil
			}
		}
	}
	return jworker.NewSimpleWorker(loop)
}
