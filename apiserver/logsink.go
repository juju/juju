// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/logsink"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

type agentLoggingStrategy struct {
	modelLogger corelogger.ModelLogger

	recordLogWriter corelogger.LogWriter
	releaser        func() error
	entity          string
	modelUUID       coremodel.UUID
}

// newAgentLogWriteCloserFunc returns a function that will create a
// logsink.LoggingStrategy given an *http.Request.
func newAgentLogWriteCloserFunc(
	ctxt httpContext,
	modelLogger corelogger.ModelLogger,
) logsink.NewLogWriteCloserFunc {
	return func(req *http.Request) (logsink.LogWriteCloser, error) {
		strategy := &agentLoggingStrategy{
			modelLogger: modelLogger,
		}
		if err := strategy.init(ctxt, req); err != nil {
			return nil, errors.Annotate(err, "initialising agent logsink session")
		}
		return strategy, nil
	}
}

func (s *agentLoggingStrategy) init(ctxt httpContext, req *http.Request) error {
	st, entity, err := ctxt.stateForRequestAuthenticatedAgent(req)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = st.Release() }()

	s.entity = entity.Tag().String()
	s.modelUUID = coremodel.UUID(st.ModelUUID())

	if s.recordLogWriter, err = s.modelLogger.GetLogWriter(req.Context(), s.modelUUID); err != nil {
		return errors.Trace(err)
	}
	s.releaser = func() error {
		return s.modelLogger.RemoveLogWriter(s.modelUUID)
	}
	return nil
}

// Close is part of the logsink.LogWriteCloser interface.
func (s *agentLoggingStrategy) Close() error {
	return s.releaser()
}

// WriteLog is part of the logsink.LogWriteCloser interface.
func (s *agentLoggingStrategy) WriteLog(m params.LogRecord) error {
	level, _ := corelogger.ParseLevelFromString(m.Level)
	return errors.Annotate(s.recordLogWriter.Log([]corelogger.LogRecord{{
		Time:      m.Time,
		Entity:    s.entity,
		Module:    m.Module,
		Location:  m.Location,
		Level:     level,
		Message:   m.Message,
		Labels:    m.Labels,
		ModelUUID: s.modelUUID.String(),
	}}), "writing model logs failed")
}
