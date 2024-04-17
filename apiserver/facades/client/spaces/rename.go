// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
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
		if fromTag.Id() == network.AlphaSpaceName {
			newErr := errors.Errorf("the %q space cannot be renamed", network.AlphaSpaceName)
			result.Results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		toTag, err := names.ParseSpaceTag(spaceRename.ToSpaceTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		toSpace, err := api.networkService.SpaceByName(ctx, toTag.Id())
		if err != nil && !errors.Is(err, errors.NotFound) {
			newErr := errors.Annotatef(err, "retrieving space %q", toTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
		if toSpace != nil {
			newErr := errors.AlreadyExistsf("space %q", toTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}

		fromSpace, err := api.networkService.SpaceByName(ctx, fromTag.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "retrieving space %q", fromTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
		if err := api.networkService.UpdateSpace(ctx, fromSpace.ID, toTag.Id()); err != nil {
			newErr := errors.Annotatef(err, "updating space %q", fromTag.Id())
			result.Results[i].Error = apiservererrors.ServerError(errors.Trace(newErr))
			continue
		}
	}
	return result, nil
}
