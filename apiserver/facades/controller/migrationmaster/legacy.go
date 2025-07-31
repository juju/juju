// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// ModelInfo returns essential information about the model to be
// migrated.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (api *APIV4) ModelInfo(ctx context.Context) (params.MigrationModelInfoLegacy, error) {
	modelInfo, err := api.API.ModelInfo(ctx)
	if err != nil {
		return params.MigrationModelInfoLegacy{}, errors.Trace(err)
	}
	owner, err := params.ApproximateUserTagFromQualifier(coremodel.Qualifier(modelInfo.Qualifier))
	if err != nil {
		return params.MigrationModelInfoLegacy{}, apiservererrors.ServerError(err)
	}

	return params.MigrationModelInfoLegacy{
		UUID:             modelInfo.UUID,
		Name:             modelInfo.Name,
		OwnerTag:         owner.String(),
		AgentVersion:     modelInfo.AgentVersion,
		ModelDescription: modelInfo.ModelDescription,
	}, nil
}
