// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

const loggerName = "juju.worker.logsender"

var logger = loggo.GetLogger(loggerName)

// New starts a logsender worker which reads log message structs from
// a channel and sends them to the JES via the logsink API.
func New(logs LogRecordCh, logSenderAPI *logsender.API) worker.Worker {
	loop := func(stop <-chan struct{}) error {
		logWriter, err := logSenderAPI.LogWriter()
		if err != nil {
			return errors.Annotate(err, "logsender dial failed")
		}
		defer logWriter.Close()
		for {
			select {
			case rec := <-logs:
				err := logWriter.WriteLog(&params.LogRecord{
					Time:     rec.Time,
					Module:   rec.Module,
					Location: rec.Location,
					Level:    rec.Level,
					Message:  rec.Message,
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
						Level:   loggo.WARNING,
						Message: fmt.Sprintf("%d log messages dropped due to lack of API connectivity", rec.DroppedAfter),
					})
					if err != nil {
						return errors.Trace(err)
					}
				}

			case <-stop:
				return nil
			}
		}
	}
	return worker.NewSimpleWorker(loop)
}
