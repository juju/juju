// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
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

// MigrationLogModelService provides the controller-scoped model lookup needed
// to establish that a log transfer request names a model that actually exists
// on this controller.
type MigrationLogModelService interface {
	// CheckModelExists returns whether the model with the given UUID exists
	// and is active on this controller.
	CheckModelExists(ctx context.Context, modelUUID coremodel.UUID) (bool, error)
}

// MigrationLogModeService reports the migration mode of the model that a log
// transfer request is for.
type MigrationLogModeService interface {
	// ModelMigrationMode returns the current migration mode for the model.
	ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error)
}

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

// migrationLogModelUUID returns the UUID of the model that a log transfer
// request is for.
//
// The model is named by the migration header, not by the request context:
// /migrate/logtransfer is a controller-scoped route, so the context carries the
// *controller* model's UUID rather than the UUID of the model being migrated.
// Reading the header here keeps it local to this handler; it must never be used
// to populate the request context, which feeds authorization and service
// resolution elsewhere.
func migrationLogModelUUID(req *http.Request) (coremodel.UUID, error) {
	uuidStr, ok := httpcontext.MigrationRequestModelUUID(req)
	if !ok {
		return "", errors.Trace(apiservererrors.ErrPerm)
	}
	modelUUID := coremodel.UUID(uuidStr)
	if err := modelUUID.Validate(); err != nil {
		return "", errors.BadRequestf("invalid migration model UUID %q", uuidStr)
	}
	return modelUUID, nil
}

// validateMigrationLogTarget checks that modelUUID names a model this
// controller can accept migrated log records for. The model must exist, and it
// must have finished importing.
func validateMigrationLogTarget(
	ctx context.Context,
	modelUUID coremodel.UUID,
	models MigrationLogModelService,
	migration MigrationLogModeService,
) error {
	exists, err := models.CheckModelExists(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	if !exists {
		return errors.NotFoundf("model %q", modelUUID)
	}

	migrationMode, err := migration.ModelMigrationMode(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	// Require MigrationModeNone because logtransfer happens after the
	// model proper is completely imported.
	if migrationMode != modelmigration.MigrationModeNone {
		return errors.BadRequestf(
			"model migration mode is %q instead of None", migrationMode)
	}
	return nil
}

func (s *migrationLoggingStrategy) init(ctxt httpContext, req *http.Request) error {
	modelUUID, err := migrationLogModelUUID(req)
	if err != nil {
		return errors.Trace(err)
	}

	// Here the log messages are expected to be coming from another
	// Juju controller, so the version number provided should be the
	// Juju version of the source controller. Require this to be
	// passed, even though we don't use it anywhere at the moment - it
	// provides future-proofing if we need to do some kind of
	// conversion of log messages from an old client.
	//
	// This is another header-only check, so it runs before any database work.
	if _, err := common.JujuClientVersionFromRequest(req); err != nil {
		return errors.Trace(err)
	}

	// Resolve the domain services for the model being migrated, as named by the
	// migration header, rather than for the model implied by the route.
	domainServices, err := ctxt.domainServicesDuringMigrationForRequest(req)
	if err != nil {
		return errors.Trace(err)
	}
	if err := validateMigrationLogTarget(
		req.Context(),
		modelUUID,
		domainServices.Model(),
		domainServices.ModelMigration(),
	); err != nil {
		return errors.Trace(err)
	}

	s.modelUUID = modelUUID

	// Obtain the log writer last. Doing so starts a logger worker keyed on the
	// model UUID, so it must only happen once the request is known to name a
	// real model that is ready to receive migrated records.
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
