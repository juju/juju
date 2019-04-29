// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
)

const subnetsFacade = "Subnets"

// API provides access to the Subnets API facade.
type API struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Subnets facade.
func NewAPI(caller base.APICallCloser) *API {
	if caller == nil {
		panic("caller is nil")
	}
	clientFacade, facadeCaller := base.NewClientFacade(caller, subnetsFacade)
	return &API{
		ClientFacade: clientFacade,
		facade:       facadeCaller,
	}
}

// AddSubnet adds an existing subnet to the model.
func (api *API) AddSubnet(subnet names.SubnetTag, providerId network.Id, space names.SpaceTag, zones []string) error {
	var response params.ErrorResults
	// Prefer ProviderId when set over CIDR.
	subnetTag := subnet.String()
	if providerId != "" {
		subnetTag = ""
	}

	params := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{
			SubnetTag:        subnetTag,
			SubnetProviderId: string(providerId),
			SpaceTag:         space.String(),
			Zones:            zones,
		}},
	}
	err := api.facade.FacadeCall("AddSubnets", params, &response)
	if err != nil {
		return errors.Trace(err)
	}
	return response.OneError()
}

// CreateSubnet creates a new subnet with the provider.
func (api *API) CreateSubnet(subnet names.SubnetTag, space names.SpaceTag, zones []string, isPublic bool) error {
	var response params.ErrorResults
	params := params.CreateSubnetsParams{
		Subnets: []params.CreateSubnetParams{{
			SubnetTag: subnet.String(),
			SpaceTag:  space.String(),
			Zones:     zones,
			IsPublic:  isPublic,
		}},
	}
	err := api.facade.FacadeCall("CreateSubnets", params, &response)
	if err != nil {
		return errors.Trace(err)
	}
	return response.OneError()
}

// ListSubnets fetches all the subnets known by the model.
func (api *API) ListSubnets(spaceTag *names.SpaceTag, zone string) ([]params.Subnet, error) {
	var response params.ListSubnetsResults
	var space string
	if spaceTag != nil {
		space = spaceTag.String()
	}
	args := params.SubnetsFilters{
		SpaceTag: space,
		Zone:     zone,
	}
	err := api.facade.FacadeCall("ListSubnets", args, &response)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return response.Results, nil
}
