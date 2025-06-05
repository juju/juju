// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/rpc/params"
)

// RenameSpace renames a space.
func (api *API) RenameSpace(ctx context.Context, args params.RenameSpacesParams) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	if err := api.ensureSpacesAreMutable(ctx); err != nil {
		return result, err
	}

	result.Results = make([]params.ErrorResult, len(args.Changes))
	for i, spaceRename := range args.Changes {
		fromTag, err := names.ParseSpaceTag(spaceRename.FromSpaceTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		fromSpaceName := network.NewSpaceName(fromTag.Id())
		if fromSpaceName == network.AlphaSpaceName {
			newErr := errors.Errorf("the %q space cannot be renamed", network.AlphaSpaceName)
			result.Results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}

		toTag, err := names.ParseSpaceTag(spaceRename.ToSpaceTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		toSpaceName := network.NewSpaceName(toTag.Id())

		toSpace, err := api.networkService.SpaceByName(ctx, toSpaceName)
		if err != nil && !errors.Is(err, networkerrors.SpaceNotFound) {
			newErr := errors.Annotatef(err, "retrieving space %q", toTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
		if toSpace != nil {
			newErr := errors.AlreadyExistsf("space %q", toTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}

		fromSpace, err := api.networkService.SpaceByName(ctx, fromSpaceName)
		if err != nil {
			newErr := errors.Annotatef(err, "retrieving space %q", fromSpaceName)
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
		if err := api.networkService.UpdateSpace(ctx, fromSpace.ID, toSpaceName); err != nil {
			newErr := errors.Annotatef(err, "updating space %q", fromSpaceName)
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
	}
	return result, nil
}
