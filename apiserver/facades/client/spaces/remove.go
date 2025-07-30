// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
)

// RemoveSpace removes a space.
// Returns SpaceResults if entities/settings are found which makes the deletion not possible.
func (api *API) RemoveSpace(ctx context.Context, spaceParams params.RemoveSpaceParams) (params.RemoveSpaceResults, error) {
	result := params.RemoveSpaceResults{}

	if err := api.ensureSpacesAreMutable(ctx); err != nil {
		return result, err
	}

	result.Results = make([]params.RemoveSpaceResult, len(spaceParams.SpaceParams))
	for i, spaceParam := range spaceParams.SpaceParams {
		spacesTag, err := names.ParseSpaceTag(spaceParam.Space.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		spaceName := network.SpaceName(spacesTag.Id())

		if !api.checkSpaceIsRemovable(ctx, i, spaceName, &result, spaceParam.Force) {
			continue
		}

		if spaceParam.DryRun {
			continue
		}

		space, err := api.networkService.SpaceByName(ctx, spaceName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		if err := api.networkService.RemoveSpace(ctx, space.ID); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
	}
	return result, nil
}

func (api *API) checkSpaceIsRemovable(
	ctx context.Context,
	index int,
	spaceName network.SpaceName,
	results *params.RemoveSpaceResults,
	force bool,
) bool {
	removable := true

	if spaceName == network.AlphaSpaceName {
		newErr := errors.New("the alpha space cannot be removed")
		results.Results[index].Error = apiservererrors.ServerError(newErr)
		return false
	}

	if force {
		return true
	}

	space, err := api.networkService.SpaceByName(ctx, spaceName)
	if err != nil {
		results.Results[index].Error = apiservererrors.ServerError(errors.Trace(err))
		return false
	}

	// TODO(gfouillet) - 2025-07-29: remove checks from here and move them into
	//   the service layer. A space cannot be removed if:
	//    - it is used as binding for application or endpoint
	//    - it is used as constraint for model or application
	//    - it is the controller management space
	bindingTags, err := api.applicationTagsForSpace(ctx, space.ID)
	if err != nil {
		results.Results[index].Error = apiservererrors.ServerError(errors.Trace(err))
		return false
	}
	if len(bindingTags) != 0 {
		results.Results[index].Bindings = convertTagsToEntities(bindingTags)
		removable = false
	}
	return removable
}

// applicationTagsForSpace returns the tags for all applications with an
// endpoint bound to a space with the input name.
func (api *API) applicationTagsForSpace(ctx context.Context, spaceID network.SpaceUUID) ([]names.Tag, error) {
	applications, err := api.applicationService.GetApplicationsBoundToSpace(ctx, spaceID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := make([]names.Tag, len(applications))
	for i, app := range applications {
		tags[i] = names.NewApplicationTag(app)
	}
	return tags, nil
}

func convertTagsToEntities(tags []names.Tag) []params.Entity {
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i].Tag = tag.String()
	}

	return entities
}
