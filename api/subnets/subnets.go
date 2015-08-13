// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.subnets")

const subnetsFacade = "Subnets"

// API provides access to the InstancePoller API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Subnets facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, subnetsFacade)
	return &API{
		facade: facadeCaller,
	}
}

func (api *API) AddSubnet(cidr, providerId, space string, zones []string) (params.ErrorResults, error) {
	var response params.ErrorResults
	spaceTag := names.NewSpaceTag(space).String()
	subnetTag := names.NewSubnetTag(cidr).String()
	params := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{
			{
				SubnetTag:        subnetTag,
				SubnetProviderId: providerId,
				SpaceTag:         spaceTag,
				Zones:            zones,
			}},
	}
	err := api.facade.FacadeCall("AddSubnets", params, &response)
	return response, err
}

func (api *API) CreateSubnet(cidr, space string, zones []string, isPublic bool) (params.ErrorResults, error) {
	var response params.ErrorResults
	spaceTag := names.NewSpaceTag(space).String()
	subnetTag := names.NewSubnetTag(cidr).String()
	params := params.CreateSubnetsParams{
		Subnets: []params.CreateSubnetParams{
			{
				SubnetTag: subnetTag,
				SpaceTag:  spaceTag,
				Zones:     zones,
				IsPublic:  isPublic,
			}},
	}
	err := api.facade.FacadeCall("CreateSubnets", params, &response)
	return response, err
}

func (api *API) ListSubnets(spaceTag names.SpaceTag, zone string) (params.ListSubnetsResults, error) {
	var response params.ListSubnetsResults
	params := params.ListSubnetsParams{
		Filters: []params.ListSubnetsFilterParams{
			{SpaceTag: spaceTag.String(), Zone: zone},
		},
	}
	err := api.facade.FacadeCall("ListSubnets", params, &response)
	return response, err
}
