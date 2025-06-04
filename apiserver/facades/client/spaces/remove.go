// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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
	space, err := api.networkService.SpaceByName(ctx, spaceName)
	if err != nil {
		results.Results[index].Error = apiservererrors.ServerError(errors.Trace(err))
		return false
	}
	bindingTags, err := api.applicationTagsForSpace(ctx, space.ID)
	if err != nil {
		results.Results[index].Error = apiservererrors.ServerError(errors.Trace(err))
		return false
	}
	constraintTags, err := api.entityTagsForSpaceConstraintsBlockingRemove(space.Name)
	if err != nil {
		results.Results[index].Error = apiservererrors.ServerError(errors.Trace(err))
		return false
	}
	settingMatches, err := api.getSpaceControllerSettings(context.Background(), space.Name)
	if err != nil {
		results.Results[index].Error = apiservererrors.ServerError(errors.Trace(err))
		return false
	}

	if force {
		return true
	}

	if len(settingMatches) != 0 {
		results.Results[index].ControllerSettings = settingMatches
		removable = false
	}
	if len(bindingTags) != 0 {
		results.Results[index].Bindings = convertTagsToEntities(bindingTags)
		removable = false
	}
	if len(constraintTags) != 0 {
		results.Results[index].Constraints = convertTagsToEntities(constraintTags)
		removable = false
	}
	return removable
}

// applicationTagsForSpace returns the tags for all applications with an
// endpoint bound to a space with the input name.
func (api *API) applicationTagsForSpace(ctx context.Context, spaceID network.SpaceUUID) ([]names.Tag, error) {
	allSpaces, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	applications, err := api.applicationsBoundToSpace(spaceID, allSpaces)
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

// entityTagsForSpaceConstraintsBlockingRemove returns tags for entities
// with constraints for the input space name, that disallow removal of the
// space. I.e. those other than units and machines.
func (api *API) entityTagsForSpaceConstraintsBlockingRemove(spaceName network.SpaceName) ([]names.Tag, error) {
	allTags, err := api.entityTagsForSpaceConstraints(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var notSkipping []names.Tag
	for _, tag := range allTags {
		if tag.Kind() == names.UnitTagKind {
			continue
		}
		if tag.Kind() == names.MachineTagKind {
			continue
		}
		notSkipping = append(notSkipping, tag)
	}
	return notSkipping, nil
}

// entityTagsForSpaceConstraints returns the tags for all entities
// with constraints that refer to the input space name.
func (api *API) entityTagsForSpaceConstraints(spaceName network.SpaceName) ([]names.Tag, error) {
	cons, err := api.backing.ConstraintsBySpaceName(spaceName.String())
	if err != nil {
		return nil, errors.Trace(err)
	}

	tags := make([]names.Tag, len(cons))
	for i, doc := range cons {
		tag := state.TagFromDocID(doc.ID())
		if tag == nil {
			return nil, errors.Errorf("Could not parse id: %q", doc.ID())
		}
		tags[i] = tag
	}
	return tags, nil
}

func (api *API) getSpaceControllerSettings(ctx context.Context, spaceName network.SpaceName) ([]string, error) {
	var matches []string

	if !api.backing.IsController() {
		return matches, nil
	}

	currentControllerConfig, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return matches, errors.Trace(err)
	}

	if mgmtSpace := currentControllerConfig.JujuManagementSpace(); mgmtSpace == spaceName {
		matches = append(matches, controller.JujuManagementSpace)
	}
	if haSpace := currentControllerConfig.JujuHASpace(); haSpace == spaceName {
		matches = append(matches, controller.JujuHASpace)
	}
	return matches, nil
}
