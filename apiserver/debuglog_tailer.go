// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/apiserver/authentication"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/logtailer"
	"github.com/juju/juju/rpc/params"
)

func newDebugLogTailerHandler(
	ctxt httpContext,
	authenticator authentication.HTTPAuthenticator,
	authorizer authentication.Authorizer,
	logDir string,
) http.Handler {
	return newDebugLogHandler(ctxt, authenticator, authorizer, logDir, handleDebugLogRequest)
}

type logTailerFunc func(logtailer.LogTailerParams) (logtailer.LogTailer, error)

func handleDebugLogRequest(
	clock clock.Clock,
	maxDuration time.Duration,
	reqParams debugLogParams,
	socket debugLogSocket,
	logTailerFunc logTailerFunc,
	stop <-chan struct{},
	stateClosing <-chan struct{},
) error {
	tailerParams := makeLogTailerParams(reqParams)
	tailer, err := logTailerFunc(tailerParams)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = worker.Stop(tailer)
	}()

	// Indicate that all is well.
	socket.sendOk()

	timeout := clock.After(maxDuration)

	var lineCount uint
	for {
		select {
		case <-stateClosing:
			return nil
		case <-stop:
			return nil
		case <-timeout:
			return nil
		case rec, ok := <-tailer.Logs():
			if !ok {
				return errors.Annotate(tailer.Wait(), "tailer stopped")
			}

			if err := socket.sendLogRecord(formatLogRecord(rec), reqParams.version); err != nil {
				return errors.Annotate(err, "sending failed")
			}

			lineCount++
			if reqParams.noTail && reqParams.initialLines > 0 && lineCount == reqParams.initialLines {
				return nil
			}
		}
	}
}

func makeLogTailerParams(reqParams debugLogParams) logtailer.LogTailerParams {
	return logtailer.LogTailerParams{
		MinLevel:      reqParams.filterLevel,
		NoTail:        reqParams.noTail,
		Firehose:      reqParams.firehose,
		StartTime:     reqParams.startTime,
		InitialLines:  int(reqParams.initialLines),
		IncludeEntity: reqParams.includeEntity,
		ExcludeEntity: reqParams.excludeEntity,
		IncludeModule: reqParams.includeModule,
		ExcludeModule: reqParams.excludeModule,
		IncludeLabels: reqParams.includeLabels,
		ExcludeLabels: reqParams.excludeLabels,
		FromTheStart:  reqParams.fromTheStart,
	}
}

func formatLogRecord(r corelogger.LogRecord) *params.LogMessage {
	return &params.LogMessage{
		ModelUUID: r.ModelUUID,
		Entity:    r.Entity,
		Timestamp: r.Time,
		Severity:  r.Level.String(),
		Module:    r.Module,
		Location:  r.Location,
		Message:   r.Message,
		Labels:    r.Labels,
	}
}
