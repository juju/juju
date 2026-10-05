// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"time"

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

const logTransferTrackingPeriod = 2 * time.Minute

type migrationLoggingStrategy struct {
	modelLogger corelogger.ModelLogger

	recordLogWriter corelogger.LogWriter

	setLastLogTransferTime func(context.Context, time.Time) error

	modelUUID      coremodel.UUID
	requestContext context.Context
	trackedTime    time.Time
}

// newMigrationLogWriteFunc returns a writer for migrated log records.
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
	domainServices, err := ctxt.domainServicesDuringMigrationForRequest(req)
	if err != nil {
		return errors.Trace(err)
	}
	migrationService := domainServices.ModelMigration()
	migrationMode, err := migrationService.ModelMigrationMode(req.Context())
	if err != nil {
		return errors.Trace(err)
	}
	// Require MigrationModeNone because logtransfer happens after the
	// model proper is completely imported.
	if migrationMode != modelmigration.MigrationModeNone {
		return errors.BadRequestf(
			"model migration mode is %q instead of None", migrationMode)
	}
	s.setLastLogTransferTime = migrationService.SetLastLogTransferTime

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

	modelUUID, valid := httpcontext.MigrationRequestModelUUID(req)
	if !valid {
		return errors.Trace(apiservererrors.ErrPerm)
	}
	s.modelUUID = coremodel.UUID(modelUUID)

	if s.recordLogWriter, err = s.modelLogger.GetLogWriter(req.Context(), s.modelUUID); err != nil {
		return errors.Trace(err)
	}

	s.requestContext = req.Context()
	return nil
}

// WriteLog implements logsink.LogWriter.
func (s *migrationLoggingStrategy) WriteLog(m params.LogRecord) error {
	level, _ := corelogger.ParseLevelFromString(m.Level)
	if err := s.recordLogWriter.Log([]corelogger.LogRecord{{
		Time:      m.Time,
		Entity:    m.Entity,
		Module:    m.Module,
		Location:  m.Location,
		Level:     level,
		Message:   m.Message,
		Labels:    m.Labels,
		ModelUUID: s.modelUUID.String(),
	}}); err != nil {
		return errors.Trace(err)
	}
	return errors.Annotate(s.trackLogTransferTime(m.Time), "tracking transferred log")
}

// trackLogTransferTime limits database checkpoint writes while logs are
// transferred. Persisting a checkpoint for every log record would slow the
// transfer. Records since the last checkpoint may be replayed after an
// interruption; these duplicates are tolerated because no logs are missed.
func (s *migrationLoggingStrategy) trackLogTransferTime(t time.Time) error {
	if t.IsZero() || t.Sub(s.trackedTime) < logTransferTrackingPeriod {
		return nil
	}
	if err := s.setLastLogTransferTime(s.requestContext, t); err != nil {
		return errors.Trace(err)
	}
	s.trackedTime = t
	return nil
}
