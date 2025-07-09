// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/rpc/params"
)

// MoveSubnets ensures that the input subnets are in the input space.
func (api *API) MoveSubnets(ctx context.Context, args params.MoveSubnetsParams) (params.MoveSubnetsResults, error) {
	var result params.MoveSubnetsResults

	if err := api.ensureSpacesAreMutable(ctx); err != nil {
		return result, err
	}

	results := make([]params.MoveSubnetsResult, len(args.Args))
	for i, toSpaceParams := range args.Args {
		// Note that although spaces have an ID, a space tag represents
		// a space *name*, which remains a unique identifier.
		// We need to retrieve the space in order to use its ID.
		spaceTag, err := names.ParseSpaceTag(toSpaceParams.SpaceTag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		if len(toSpaceParams.SubnetTags) == 0 {
			results[i].Error = apiservererrors.ServerError(errors.New("no subnets specified"))
			continue
		}
		spaceName := network.SpaceName(spaceTag.Id())
		subnets, err := transform.SliceOrErr(toSpaceParams.SubnetTags, func(f string) (domainnetwork.SubnetUUID,
			error) {
			subnetTag, err := names.ParseSubnetTag(f)
			if err != nil {
				return "", errors.Trace(err)
			}
			result := domainnetwork.SubnetUUID(subnetTag.Id())
			return result, errors.Trace(result.Validate())
		})
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		subnetInfos, err := api.networkService.GetAllSubnets(ctx)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		movedSubnets, err := api.networkService.MoveSubnetsToSpace(ctx, subnets, spaceName, toSpaceParams.Force)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		results[i].NewSpaceTag = spaceTag.String()
		results[i].MovedSubnets = transform.Slice(movedSubnets, func(f domainnetwork.MovedSubnets) params.MovedSubnet {
			return params.MovedSubnet{
				SubnetTag:   names.NewSubnetTag(f.UUID.String()).String(),
				OldSpaceTag: names.NewSpaceTag(f.FromSpace.String()).String(),
				CIDR:        subnetInfos.GetByID(network.Id(f.UUID)).CIDR,
			}
		})
	}

	result.Results = results
	return result, nil
}
