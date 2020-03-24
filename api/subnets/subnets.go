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

// CreateSubnet creates a new subnet with the provider.
func (api *API) CreateSubnet(subnet names.SubnetTag, space names.SpaceTag, zones []string, isPublic bool) error {
	// TODO (hml) 2019-08-23
	// This call is behind a feature flag and panics due to lack of
	// facade on the the other end.  It's in the list to be audited
	// for updates as part of current networking improvements.  Fix
	// names.v2 SubnetTag at that time.
	//
	// TODO (stickupkid): This should be safe to remove, as it's behind a
	// feature that's been removed and there is no api-server implementation
	// that handles this call?
	return errors.NewNotImplemented(nil, "CreateSubnet")
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
