// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/export"
	migrationv2 "github.com/juju/juju/internal/migration/v2"
	"github.com/juju/juju/rpc/params"
)

// ImportModelV2 applies a v8 migration envelope's controller-scoped semantic
// data to the target controller. See [migrationv2.ImportModel] for the
// orchestration; this method only resolves the migration scope for the
// envelope's model UUID and delegates.
//
// If a claim already exists for envelope.ModelInfo.UUID, the returned error
// wraps [coreerrors.AlreadyExists] (phase-specific wording is supplied by the
// modelmigration domain).
func (i *ModelImporter) ImportModelV2(
	ctx context.Context, envelope params.SerializedModelV2, view export.ProjectionView,
) error {
	modelUUID := coremodel.UUID(envelope.ModelInfo.UUID)
	scope := i.scope(modelUUID)

	return migrationv2.ImportModel(ctx, migrationv2.Deps{
		ControllerDB: scope.ControllerDB(),
		ModelDB:      scope.ModelDB(),
		Clock:        i.clock,
		Logger:       i.logger,
	}, envelope, view)
}
