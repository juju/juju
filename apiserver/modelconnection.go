// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
)

// modelConnection describes whether the API server may serve connections for a
// model, and what the resulting API root has to know about it.
type modelConnection struct {
	// connectable reports whether connections may be served for the model at
	// all. The remaining fields are only meaningful when it is true.
	connectable bool

	// modelName is the human friendly name of the model.
	modelName string

	// modelType is the type of the model.
	modelType coremodel.ModelType

	// migrationMode is the model's current migration mode, which decides how
	// far the resulting API root is restricted.
	migrationMode modelmigration.MigrationMode
}

// modelConnectionFor reports whether the API server may serve connections for
// the given model.
//
// An activated model is connectable, as it always has been. A model that has
// not been activated is connectable only while a migration is importing it:
// during the migration's VALIDATION phase every agent in the model has to dial
// this controller, log in, and confirm it can be served here, and that happens
// before the import is committed and the model activated. Refusing those logins
// fails the phase and aborts the migration.
//
// This does not widen user access. A user login to a model whose migration mode
// is importing is rejected by restrictAPIRootDuringMaintenance, which is the
// same treatment an activated model being imported has always had.
//
// A model that does not exist, or one left half built for any other reason -
// an add-model that never completed, say - is not connectable.
func modelConnectionFor(
	ctx context.Context,
	modelService ModelService,
	migrationService ModelMigrationService,
	modelUUID coremodel.UUID,
) (modelConnection, error) {
	presence, err := modelService.GetModelPresence(ctx, modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		return modelConnection{}, nil
	} else if err != nil {
		return modelConnection{}, errors.Capture(err)
	}

	// The mode is read for activated models too: an exporting model restricts
	// its user logins, so the caller needs it either way.
	mode, err := migrationService.ModelMigrationMode(ctx)
	if err != nil {
		return modelConnection{}, errors.Capture(err)
	}

	if !presence.Activated && mode != modelmigration.MigrationModeImporting {
		return modelConnection{}, nil
	}

	return modelConnection{
		connectable:   true,
		modelName:     presence.Name,
		modelType:     presence.ModelType,
		migrationMode: mode,
	}, nil
}
