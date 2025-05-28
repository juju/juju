// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/rpc/params"
)

// APIV4 implements the API V4.
type APIV4 struct {
	*API
}

// ModelInfo returns essential information about the model to be
// migrated.
func (api *APIV4) ModelInfo(ctx context.Context) (params.MigrationModelInfoLegacy, error) {
	modelInfo, err := api.API.ModelInfo(ctx)
	if err != nil {

		return params.MigrationModelInfoLegacy{}, errors.Trace(err)
	}

	return params.MigrationModelInfoLegacy{
		UUID:             modelInfo.UUID,
		Name:             modelInfo.Name,
		OwnerTag:         names.NewUserTag(modelInfo.Qualifier).String(),
		AgentVersion:     modelInfo.AgentVersion,
		ModelDescription: modelInfo.ModelDescription,
	}, nil
}
