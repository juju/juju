// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// SupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func SupportsSpaces(backing environs.EnvironConfigGetter, ctx context.ProviderCallContext) error {
	env, err := environs.GetEnviron(backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}
	if !environs.SupportsSpaces(ctx, env) {
		return errors.NotSupportedf("spaces")
	}
	return nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func CreateSpaces(backing NetworkBacking, ctx context.ProviderCallContext, args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	err = SupportsSpaces(backing, ctx)
	if err != nil {
		return results, common.ServerError(errors.Trace(err))
	}

	results.Results = make([]params.ErrorResult, len(args.Spaces))

	for i, space := range args.Spaces {
		err := createOneSpace(backing, space)
		if err == nil {
			continue
		}
		results.Results[i].Error = common.ServerError(errors.Trace(err))
	}

	return results, nil
}

func createOneSpace(backing NetworkBacking, args params.CreateSpaceParams) error {
	// Validate the args, assemble information for api.backing.AddSpaces
	var subnets []string

	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}

	for _, tag := range args.SubnetTags {
		subnetTag, err := names.ParseSubnetTag(tag)
		if err != nil {
			return errors.Trace(err)
		}
		subnets = append(subnets, subnetTag.Id())
	}

	// Add the validated space.
	err = backing.AddSpace(spaceTag.Id(), network.Id(args.ProviderId), subnets, args.Public)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
