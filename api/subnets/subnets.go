// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

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
func (api *API) AddSubnet(cidr string, providerId network.Id, space names.SpaceTag, zones []string) error {
	var response params.ErrorResults
	// Prefer ProviderId when set over CIDR.
	if providerId != "" {
		cidr = ""
	}

	var args interface{}
	if bestVer := api.BestAPIVersion(); bestVer < 3 {
		args = makeAddSubnetsParamsV2(cidr, providerId, space, zones)
	} else {
		args = makeAddSubnetsParams(cidr, providerId, space, zones)
	}
	err := api.facade.FacadeCall("AddSubnets", args, &response)
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

// SubnetsByCIDR returns the collection of subnets matching each CIDR in the
// input.
func (api *API) SubnetsByCIDR(cidrs []string) ([]params.SubnetsResult, error) {
	args := params.CIDRParams{CIDRS: cidrs}

	var result params.SubnetsResults
	if err := api.facade.FacadeCall("SubnetsByCIDR", args, &result); err != nil {
		if params.IsCodeNotSupported(err) {
			return nil, errors.NewNotSupported(nil, err.Error())
		}
		return nil, errors.Trace(err)
	}

	for _, result := range result.Results {
		if result.Error != nil {
			return nil, errors.Trace(result.Error)
		}
	}

	return result.Results, nil
}

func makeAddSubnetsParamsV2(cidr string, providerId network.Id, space names.SpaceTag, zones []string) params.AddSubnetsParamsV2 {
	var subnetTag string
	if cidr != "" {
		subnetTag = "subnet-" + cidr
	}
	return params.AddSubnetsParamsV2{
		Subnets: []params.AddSubnetParamsV2{{
			SubnetTag:        subnetTag,
			SubnetProviderId: string(providerId),
			SpaceTag:         space.String(),
			Zones:            zones,
		}},
	}
}

func makeAddSubnetsParams(cidr string, providerId network.Id, space names.SpaceTag, zones []string) params.AddSubnetsParams {
	return params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{
			CIDR:             cidr,
			SubnetProviderId: string(providerId),
			SpaceTag:         space.String(),
			Zones:            zones,
		}},
	}
}
