// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
)

// ModelService defines the subset of model.Service used to check model
// connection information and redirection.
type ModelService interface {
	// GetModelConnectionInfo returns the model's type, activation state and
	// target-side import-claim presence, regardless of whether the model has
	// been activated.
	GetModelConnectionInfo(ctx context.Context, modelUUID coremodel.UUID) (model.ModelConnectionInfo, error)
	// ModelRedirection returns the model redirection information
	// for the given model UUID.
	ModelRedirection(ctx context.Context, modelUUID coremodel.UUID) (model.ModelRedirection, error)
}

// ModelMigrationService describes the migration state of the model a connection
// is being served for.
type ModelMigrationService interface {
	// ModelMigrationMode returns the current migration mode for the model.
	ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error)
}

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

	// hasImportClaim reports whether a live target-side import claim exists. It
	// is cached with the other connection information so login does not repeat
	// the controller database query made by the websocket gate.
	hasImportClaim bool

	// migrationMode is the model's current migration mode, which decides how
	// far the resulting API root is restricted.
	migrationMode modelmigration.MigrationMode
}

// modelIsConnectable reports whether the API server may serve connections for
// the given model without querying its migration mode.
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
func modelIsConnectable(
	ctx context.Context,
	modelService ModelService,
	modelUUID coremodel.UUID,
) (modelConnection, error) {
	info, err := modelService.GetModelConnectionInfo(ctx, modelUUID)
	if err != nil {
		return modelConnection{}, errors.Capture(err)
	}

	// Activation and the import claim are read atomically. This prevents the
	// activation handoff from producing the impossible-looking combination of
	// an unactivated model without an import claim.
	return modelConnection{
		connectable:    info.Activated || info.HasImportClaim,
		modelName:      info.Name,
		modelType:      info.ModelType,
		hasImportClaim: info.HasImportClaim,
	}, nil
}

// modelConnectionFor completes the cached connection information with the
// migration mode needed to restrict the authenticated API root.
func modelConnectionFor(
	ctx context.Context,
	migrationService ModelMigrationService,
	conn modelConnection,
) (modelConnection, error) {
	if !conn.connectable {
		return conn, nil
	}
	if conn.hasImportClaim {
		conn.migrationMode = modelmigration.MigrationModeImporting
		return conn, nil
	}

	// An activated model with no target-side import claim may still be
	// exporting, in which case user logins must be restricted.
	mode, err := migrationService.ModelMigrationMode(ctx)
	if err != nil {
		return modelConnection{}, errors.Capture(err)
	}
	conn.migrationMode = mode
	return conn, nil
}
