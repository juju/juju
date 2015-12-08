// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

func init() {
	common.RegisterStandardFacade("DiscoverSpaces", 0, NewDiscoverSpacesAPI)
}

// DiscoverSpacesAPI implements the API used by the discoverspaces worker.
type DiscoverSpacesAPI struct {
	*common.EnvironWatcher

	st             *state.State
	resources      *common.Resources
	authorizer     common.Authorizer
	StateAddresser *common.StateAddresser
}

// NewDiscoverSpacesAPI creates a new instance of the DiscoverSpaces API.
func NewDiscoverSpacesAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*DiscoverSpacesAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &DiscoverSpacesAPI{
		EnvironWatcher: common.NewEnvironWatcher(st, resources, authorizer),
		st:             st,
		authorizer:     authorizer,
		resources:      resources,
		StateAddresser: common.NewStateAddresser(st),
	}, nil
}

// API methods needed: ListSpaces, AddSpace, AddSubnet

// AddSpaces creates a new Juju network space.
func (api *DiscoverSpacesAPI) AddSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	results.Results = make([]params.ErrorResult, len(args.Spaces))

	for i, space := range args.Spaces {
		err := api.createOneSpace(space)
		if err == nil {
			continue
		}
		results.Results[i].Error = common.ServerError(errors.Trace(err))
	}

	return results, nil
}

func (api *DiscoverSpacesAPI) createOneSpace(args params.CreateSpaceParams) error {
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

	// Add the validated space
	_, err = api.st.AddSpace(spaceTag.Id(), "", subnets, args.Public)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func backingSubnetToParamsSubnet(subnet state.Subnet) params.Subnet {
	cidr := subnet.CIDR()
	vlantag := subnet.VLANTag()
	providerid := subnet.ProviderId()
	zone := subnet.AvailabilityZone()
	status := subnet.Status()
	var spaceTag names.SpaceTag
	if subnet.SpaceName() != "" {
		spaceTag = names.NewSpaceTag(subnet.SpaceName())
	}

	return params.Subnet{
		CIDR:       cidr,
		VLANTag:    vlantag,
		ProviderId: string(providerid),
		Zones:      []string{zone},
		Status:     status,
		SpaceTag:   spaceTag.String(),
		Life:       subnet.Life(),
	}
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *DiscoverSpacesAPI) ListSpaces() (results params.DiscoverSpacesResults, err error) {
	spaces, err := api.st.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.ProviderSpace, len(spaces))
	for i, space := range spaces {
		result := params.ProviderSpace{}
		result.ProviderId = space.ProviderId()

		subnets, err := space.Subnets()
		if err != nil {
			err = errors.Annotatef(err, "fetching subnets")
			result.Error = common.ServerError(err)
			results.Results[i] = result
			continue
		}

		result.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Subnets[i] = backingSubnetToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}
