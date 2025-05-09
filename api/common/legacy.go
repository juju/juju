// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

func (c *ModelStatusAPI) modelStatusCompat(ctx context.Context, tags ...names.ModelTag) ([]base.ModelStatus, error) {
	result := params.ModelStatusResultsLegacy{}
	models := make([]params.Entity, len(tags))
	for i, tag := range tags {
		models[i] = params.Entity{Tag: tag.String()}
	}
	req := params.Entities{
		Entities: models,
	}
	if err := c.facade.FacadeCall(ctx, "ModelStatus", req, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != len(tags) {
		return nil, errors.Errorf("%d results, expected %d", len(result.Results), len(tags))
	}
	statusResults := make([]params.ModelStatus, len(result.Results))
	for i, r := range result.Results {
		if r.Error != nil {
			statusResults[i].Error = r.Error
			continue
		}
		owner, err := names.ParseUserTag(r.OwnerTag)
		if err != nil {
			statusResults[i].Error = &params.Error{
				Message: err.Error(),
			}
			continue
		}
		statusResults[i] = params.ModelStatus{
			ModelTag:           r.ModelTag,
			Life:               r.Life,
			Type:               r.Type,
			HostedMachineCount: r.HostedMachineCount,
			ApplicationCount:   r.ApplicationCount,
			UnitCount:          r.UnitCount,
			Namespace:          owner.String(),
			Applications:       r.Applications,
			Machines:           r.Machines,
			Volumes:            r.Volumes,
			Filesystems:        r.Filesystems,
		}
	}
	return c.processModelStatusResults(statusResults)
}
