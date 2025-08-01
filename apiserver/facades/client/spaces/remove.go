// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	domainerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// RemoveSpace removes a space.
// Returns SpaceResults if entities/settings are found which makes the deletion not possible.
func (api *API) RemoveSpace(ctx context.Context, spaceParams params.RemoveSpaceParams) (params.RemoveSpaceResults, error) {
	result := params.RemoveSpaceResults{}

	if err := api.ensureSpacesAreMutable(ctx); err != nil {
		return result, err
	}

	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Errorf("retrieving controller config: %w", err)
	}
	mgtSpace := cfg.JujuManagementSpace()

	result.Results = make([]params.RemoveSpaceResult, len(spaceParams.SpaceParams))
	for i, spaceParam := range spaceParams.SpaceParams {
		spacesTag, err := names.ParseSpaceTag(spaceParam.Space.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Capture(err))
			continue
		}
		spaceName := network.SpaceName(spacesTag.Id())

		if spaceName == network.AlphaSpaceName {
			result.Results[i].Error = apiservererrors.ServerError(errors.Errorf("the %q space cannot be removed", network.AlphaSpaceName))
			continue
		}

		// Check that the space is not the juju controller space
		isMgtSpace := spaceName == mgtSpace

		// RemoveSpace allows to both get violation and remove space.
		// We use dryRun (get violation without change) if asked or if we are
		// in the controller management space, since it would be a violation to
		// remove the controller management space.
		// However, RemoveSpace with the dry run flag in this case allows
		// fetching any other violation.
		violations, err := api.networkService.RemoveSpace(ctx, spaceName, spaceParam.Force, spaceParam.DryRun || isMgtSpace)
		if errors.Is(err, domainerrors.SpaceNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(domainerrors.SpaceNotFound)
			continue
		}
		if err != nil {
			return result, errors.Errorf("removing space %q: %w", spaceName, err)
		}
		if spaceParam.Force {
			// We do not publish the violation if forced.
			continue
		}
		toAppEntity := func(f string) params.Entity {
			return params.Entity{Tag: names.NewApplicationTag(f).String()}
		}
		result.Results[i].Bindings = transform.Slice(violations.ApplicationBindings, toAppEntity)
		result.Results[i].Constraints = transform.Slice(violations.ApplicationConstraints, toAppEntity)
		if violations.HasModelConstraint {
			result.Results[i].Constraints = append(result.Results[i].Constraints, params.Entity{Tag: api.modelTag.String()})
		}
		if isMgtSpace {
			result.Results[i].ControllerSettings = []string{controller.JujuManagementSpace}
		}

	}
	return result, nil
}
