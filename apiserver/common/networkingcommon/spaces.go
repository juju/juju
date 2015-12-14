// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
)

// SupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func SupportsSpaces(backing NetworkBacking) error {
	config, err := backing.EnvironConfig()
	if err != nil {
		return errors.Annotate(err, "getting environment config")
	}
	env, err := environs.New(config)
	if err != nil {
		return errors.Annotate(err, "validating environment config")
	}
	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		return errors.NotSupportedf("networking")
	}
	ok, err = netEnv.SupportsSpaces()
	if !ok {
		return errors.NotSupportedf("spaces")
	}
	return err
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func CreateSpaces(backing NetworkBacking, args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	err = SupportsSpaces(backing)
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
	err = backing.AddSpace(spaceTag.Id(), args.ProviderId, subnets, args.Public)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
