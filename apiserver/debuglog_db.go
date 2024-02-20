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
	"github.com/juju/juju/state"
)

func newDebugLogDBHandler(
	ctxt httpContext,
	authenticator authentication.HTTPAuthenticator,
	authorizer authentication.Authorizer,
	logDir string,
) http.Handler {
	return newDebugLogHandler(ctxt, authenticator, authorizer, logDir, handleDebugLogDBRequest)
}

func handleDebugLogDBRequest(
	clock clock.Clock,
	maxDuration time.Duration,
	st state.LogTailerState,
	reqParams debugLogParams,
	socket debugLogSocket,
	logDir string,
	stop <-chan struct{},
) error {
	tailerParams := makeLogTailerParams(reqParams)
	tailer, err := newLogTailer(st, logDir, tailerParams)
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
		case <-stop:
			return nil
		case <-timeout:
			return nil
		case rec, ok := <-tailer.Logs():
			if !ok {
				return errors.Annotate(tailer.Wait(), "tailer stopped")
			}

			if err := socket.sendLogRecord(formatLogRecord(rec)); err != nil {
				return errors.Annotate(err, "sending failed")
			}

			lineCount++
			if reqParams.maxLines > 0 && lineCount == reqParams.maxLines {
				return nil
			}
		}
	}
}

func makeLogTailerParams(reqParams debugLogParams) logtailer.LogTailerParams {
	tailerParams := logtailer.LogTailerParams{
		MinLevel:      reqParams.filterLevel,
		NoTail:        reqParams.noTail,
		StartTime:     reqParams.startTime,
		InitialLines:  int(reqParams.backlog),
		IncludeEntity: reqParams.includeEntity,
		ExcludeEntity: reqParams.excludeEntity,
		IncludeModule: reqParams.includeModule,
		ExcludeModule: reqParams.excludeModule,
		IncludeLabel:  reqParams.includeLabel,
		ExcludeLabel:  reqParams.excludeLabel,
	}
	if reqParams.fromTheStart {
		tailerParams.InitialLines = 0
	}
	return tailerParams
}

func formatLogRecord(r *corelogger.LogRecord) *params.LogMessage {
	return &params.LogMessage{
		Entity:    r.Entity,
		Timestamp: r.Time,
		Severity:  r.Level.String(),
		Module:    r.Module,
		Location:  r.Location,
		Message:   r.Message,
		Labels:    r.Labels,
	}
}

var newLogTailer = _newLogTailer // For replacing in tests

func _newLogTailer(st state.LogTailerState, logDir string, params logtailer.LogTailerParams) (logtailer.LogTailer, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelOwnerAndName := corelogger.ModelFilePrefix(m.Owner().Id(), m.Name())
	return logtailer.NewLogTailer(st.ModelUUID(), corelogger.ModelLogFile(logDir, st.ModelUUID(), modelOwnerAndName), params)
}
