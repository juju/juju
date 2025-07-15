// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/logsink"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/rpc/params"
)

type migrationLoggingStrategy struct {
	modelLogger corelogger.ModelLogger

	recordLogWriter corelogger.LogWriter

	modelUUID coremodel.UUID
}

// newMigrationLogWriteFunc returns a function that will create a
// logsink.LoggingStrategy given an *http.Request, that writes log
// messages to the state database and tracks their migration.
func newMigrationLogWriteFunc(ctxt httpContext, modelLogger corelogger.ModelLogger) logsink.NewLogWriteFunc {
	return func(req *http.Request) (logsink.LogWriter, error) {
		strategy := &migrationLoggingStrategy{modelLogger: modelLogger}
		if err := strategy.init(ctxt, req); err != nil {
			return nil, errors.Annotate(err, "initialising migration logsink session")
		}
		return strategy, nil
	}
}

func (s *migrationLoggingStrategy) init(ctxt httpContext, req *http.Request) error {
	domainServices, err := ctxt.domainServicesForRequest(req.Context())
	if err != nil {
		return errors.Trace(err)
	}
	migrationMode, err := domainServices.ModelMigration().ModelMigrationMode(req.Context())
	if err != nil {
		return errors.Trace(err)
	}
	// Require MigrationModeNone because logtransfer happens after the
	// model proper is completely imported.
	if migrationMode != modelmigration.MigrationModeNone {
		return errors.BadRequestf(
			"model migration mode is %q instead of None", migrationMode)
	}

	// Here the log messages are expected to be coming from another
	// Juju controller, so the version number provided should be the
	// Juju version of the source controller. Require this to be
	// passed, even though we don't use it anywhere at the moment - it
	// provides future-proofing if we need to do some kind of
	// conversion of log messages from an old client.
	_, err = common.JujuClientVersionFromRequest(req)
	if err != nil {
		return errors.Trace(err)
	}

	modelUUID, valid := httpcontext.RequestModelUUID(req.Context())
	if !valid {
		return errors.Trace(apiservererrors.ErrPerm)
	}
	s.modelUUID = coremodel.UUID(modelUUID)

	if s.recordLogWriter, err = s.modelLogger.GetLogWriter(req.Context(), s.modelUUID); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WriteLog is part of the logsink.LogWriteCloser interface.
func (s *migrationLoggingStrategy) WriteLog(m params.LogRecord) error {
	level, _ := corelogger.ParseLevelFromString(m.Level)
	return s.recordLogWriter.Log([]corelogger.LogRecord{{
		Time:      m.Time,
		Entity:    m.Entity,
		Module:    m.Module,
		Location:  m.Location,
		Level:     level,
		Message:   m.Message,
		Labels:    m.Labels,
		ModelUUID: s.modelUUID.String(),
	}})
}
