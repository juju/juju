// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// APIV4 implements the APIV4.
type APIV4 struct {
	*API
}

// Prechecks ensure that the target controller is ready to accept a
// model migration.
func (api *APIV4) Prechecks(ctx context.Context, model params.MigrationModelInfoLegacy) error {
	ownerTag, err := names.ParseUserTag(model.OwnerTag)
	if err != nil {
		return errors.Errorf("cannot parse model %q owner during prechecks: %w", model.UUID, err)
	}
	info := params.MigrationModelInfo{
		UUID:                   model.UUID,
		Name:                   model.Name,
		Namespace:              ownerTag.Id(),
		AgentVersion:           model.AgentVersion,
		ControllerAgentVersion: model.ControllerAgentVersion,
		FacadeVersions:         model.FacadeVersions,
		ModelDescription:       model.ModelDescription,
	}
	return api.API.Prechecks(ctx, info)
}
