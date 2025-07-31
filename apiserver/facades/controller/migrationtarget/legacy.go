// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"

	"github.com/juju/names/v6"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Prechecks ensure that the target controller is ready to accept a
// model migration.
// It adapts incoming model info which uses an owner name to use a model qualifier.
func (api *APIV5) Prechecks(ctx context.Context, model params.MigrationModelInfoLegacy) error {
	ownerTag, err := names.ParseUserTag(model.OwnerTag)
	if err != nil {
		return errors.Errorf("cannot parse model %q owner during prechecks: %w", model.UUID, err)
	}
	info := params.MigrationModelInfo{
		UUID:                   model.UUID,
		Name:                   model.Name,
		Qualifier:              coremodel.QualifierFromUserTag(ownerTag).String(),
		AgentVersion:           model.AgentVersion,
		ControllerAgentVersion: model.ControllerAgentVersion,
		FacadeVersions:         model.FacadeVersions,
		ModelDescription:       model.ModelDescription,
	}
	return api.API.Prechecks(ctx, info)
}
