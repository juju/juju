// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
)

// ModelInfo return basic information about the model to migrated.
func (c *Client) modelInfoCompat(ctx context.Context) (migration.ModelInfo, error) {
	var info params.MigrationModelInfoLegacy
	err := c.caller.FacadeCall(ctx, "ModelInfo", nil, &info)
	if err != nil {
		return migration.ModelInfo{}, errors.Trace(err)
	}

	owner, err := names.ParseUserTag(info.OwnerTag)
	if err != nil {
		return migration.ModelInfo{}, errors.Trace(err)
	}

	// The model description is marshalled into YAML (description package does
	// not support JSON) to prevent potential issues with
	// marshalling/unmarshalling on the target API controller.
	var modelDescription description.Model
	if bytes := info.ModelDescription; len(bytes) > 0 {
		var err error
		modelDescription, err = description.Deserialize(info.ModelDescription)
		if err != nil {
			return migration.ModelInfo{}, errors.Annotate(err, "failed to marshal model description")
		}
	}

	return migration.ModelInfo{
		UUID:                   info.UUID,
		Name:                   info.Name,
		Namespace:              owner.Id(),
		AgentVersion:           info.AgentVersion,
		ControllerAgentVersion: info.ControllerAgentVersion,
		ModelDescription:       modelDescription,
	}, nil
}
