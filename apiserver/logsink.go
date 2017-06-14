// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type agentLoggingStrategy struct {
	fileLogger io.Writer

	st         *state.State
	releaser   func()
	version    version.Number
	entity     names.Tag
	filePrefix string
	dbLogger   *state.DbLogger
}

// newAgentLogWriteCloserFunc returns a function that will create a
// logsink.LoggingStrategy given an *http.Request, that writes log
// messages to the given writer and also to the state database.
func newAgentLogWriteCloserFunc(
	ctxt httpContext,
	fileLogger io.Writer,
) logsink.NewLogWriteCloserFunc {
	return func(req *http.Request) (logsink.LogWriteCloser, error) {
		strategy := &agentLoggingStrategy{fileLogger: fileLogger}
		if err := strategy.init(ctxt, req); err != nil {
			return nil, errors.Annotate(err, "initialising agent logsink session")
		}
		return strategy, nil
	}
}

func (s *agentLoggingStrategy) init(ctxt httpContext, req *http.Request) error {
	st, releaser, entity, err := ctxt.stateForRequestAuthenticatedAgent(req)
	if err != nil {
		return errors.Trace(err)
	}
	// Note that this endpoint is agent-only. Thus the only
	// callers will necessarily provide their Juju version.
	//
	// This would be a problem if non-Juju clients (e.g. the
	// GUI) could use this endpoint since we require that the
	// *Juju* version be provided as part of the request. Any
	// attempt to open this endpoint to broader access must
	// address this caveat appropriately.
	ver, err := logsink.JujuClientVersionFromRequest(req)
	if err != nil {
		releaser()
		return errors.Trace(err)
	}
	s.releaser = releaser
	s.version = ver
	s.entity = entity.Tag()
	s.filePrefix = st.ModelUUID() + ":"
	s.dbLogger = state.NewDbLogger(st)
	return nil
}

// WriteLog is part of the logsink.LogWriteCloser interface.
func (s *agentLoggingStrategy) WriteLog(m params.LogRecord) error {
	level, _ := loggo.ParseLevel(m.Level)
	dbErr := errors.Annotate(s.dbLogger.Log([]state.LogRecord{{
		Time:     m.Time,
		Entity:   s.entity,
		Version:  s.version,
		Module:   m.Module,
		Location: m.Location,
		Level:    level,
		Message:  m.Message,
	}}), "logging to DB failed")

	m.Entity = s.entity.String()
	fileErr := errors.Annotate(
		logToFile(s.fileLogger, s.filePrefix, m),
		"logging to logsink.log failed",
	)
	err := dbErr
	if err == nil {
		err = fileErr
	} else if fileErr != nil {
		err = errors.Errorf("%s; %s", dbErr, fileErr)
	}
	return err
}

// logToFile writes a single log message to the logsink log file.
func logToFile(writer io.Writer, prefix string, m params.LogRecord) error {
	_, err := writer.Write([]byte(strings.Join([]string{
		prefix,
		m.Entity,
		m.Time.In(time.UTC).Format("2006-01-02 15:04:05"),
		m.Level,
		m.Module,
		m.Location,
		m.Message,
	}, " ") + "\n"))
	return err
}

// Close is part of the logsink.LogWriteCloser interface. Close closes
// the DB logger and releases the state. It doesn't close the file logger
// because that lives longer than one request.
func (s *agentLoggingStrategy) Close() error {
	s.dbLogger.Close()
	s.releaser()
	return nil
}
