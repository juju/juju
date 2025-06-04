// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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
		spaceName := network.SpaceName(spaceTag.Id())

		subnets, err := api.getSubnets(ctx, toSpaceParams.SubnetTags)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		if err := api.ensureSubnetsCanBeMoved(ctx, subnets, spaceName, toSpaceParams.Force); err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		// Prepare the resulting API response before updating the
		// subnets.
		movedSubnetResult := paramsFromMovedSubnet(subnets)

		if err := api.updateSubnets(ctx, spaceName, subnets); err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}

		results[i].NewSpaceTag = spaceTag.String()
		results[i].MovedSubnets = movedSubnetResult
	}

	result.Results = results
	return result, nil
}

// getSubnets acquires all the subnets that we have
// been requested to relocate, identified by their tags.
func (api *API) getSubnets(ctx context.Context, tags []string) (network.SubnetInfos, error) {
	subnets := make(network.SubnetInfos, len(tags))
	for i, tag := range tags {
		subnetTag, err := names.ParseSubnetTag(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnet, err := api.networkService.Subnet(ctx, subnetTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnets[i] = *subnet
	}
	return subnets, nil
}

// updateSubnet updates the space in each subnet of the provided list of
// subnets.
func (api *API) updateSubnets(ctx context.Context, spaceName network.SpaceName, subnets network.SubnetInfos) error {
	space, err := api.networkService.SpaceByName(ctx, spaceName)
	if err != nil {
		return errors.Trace(err)
	}
	for _, subnet := range subnets {
		if err := api.networkService.UpdateSubnet(ctx, subnet.ID.String(), space.ID); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// ensureSubnetsCanBeMoved gathers the relevant networking info required to
// determine the validity of constraints and endpoint bindings resulting from
// a relocation of subnets.
// An error is returned if validity is violated and force is passed as false.
func (api *API) ensureSubnetsCanBeMoved(ctx context.Context, subnets network.SubnetInfos, spaceName network.SpaceName, force bool) error {
	allSpaces, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	affected, err := api.getAffectedNetworks(ctx, subnets, spaceName, allSpaces, force)
	if err != nil {
		return errors.Annotate(err, "determining affected networks")
	}

	if err := api.ensureSpaceConstraintIntegrity(ctx, affected); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(api.ensureEndpointBindingsIntegrity(ctx, affected, allSpaces))
}

// getAffectedNetworks interrogates machines connected to moving subnets.
// From these it generates lists of common unit/subnet-topologies,
// grouped by application.
func (api *API) getAffectedNetworks(ctx context.Context, subnets network.SubnetInfos, spaceName network.SpaceName, allSpaces network.SpaceInfos, force bool) (*affectedNetworks, error) {
	movingSubnetIDs := network.MakeIDSet()
	for _, subnet := range subnets {
		movingSubnetIDs.Add(subnet.ID)
	}

	affected, err := newAffectedNetworks(api.applicationService, movingSubnetIDs, spaceName, allSpaces, force, api.logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machines, err := api.backing.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := affected.processMachines(ctx, machines); err != nil {
		return nil, errors.Annotate(err, "processing machine networks")
	}

	return affected, nil
}

// ensureSpaceConstraintIntegrity identifies all applications connected to
// subnets that we have been asked to move.
// It then compares any space constraints that these applications have against
// the requested destination space, to check if they will have continuity of
// those constraints after subnet relocation.
// If force is true we only log a warning for violations, otherwise an error
// is returned.
func (api *API) ensureSpaceConstraintIntegrity(ctx context.Context, affected *affectedNetworks) error {
	constraints, err := api.backing.AllConstraints()
	if err != nil {
		return errors.Trace(err)
	}

	// Create a lookup of constrained space names by application.
	spaceConsByApp := make(map[string]set.Strings)
	for _, cons := range constraints {
		// Get the tag for the entity to which this constraint applies.
		tag := state.TagFromDocID(cons.ID())
		if tag == nil {
			return errors.Errorf("unable to determine an entity to which constraint %q applies", cons.ID())
		}

		// We only care if this is an application constraint,
		// and it includes spaces.
		val := cons.Value()
		if tag.Kind() == names.ApplicationTagKind && val.HasSpaces() {
			spaceCons := val.Spaces
			spaceConsByApp[tag.Id()] = set.NewStrings(*spaceCons...)
		}
	}

	return errors.Trace(affected.ensureConstraintIntegrity(ctx, spaceConsByApp))
}

func (api *API) ensureEndpointBindingsIntegrity(ctx context.Context, affected *affectedNetworks, allSpaces network.SpaceInfos) error {
	allBindings, err := api.backing.AllEndpointBindings()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(affected.ensureBindingsIntegrity(ctx, allBindings))
}

func paramsFromMovedSubnet(subnets network.SubnetInfos) []params.MovedSubnet {
	results := make([]params.MovedSubnet, len(subnets))
	for i, v := range subnets {
		results[i] = params.MovedSubnet{
			SubnetTag:   names.NewSubnetTag(v.ID.String()).String(),
			OldSpaceTag: names.NewSpaceTag(v.SpaceName.String()).String(),
			CIDR:        v.CIDR,
		}
	}
	return results
}
