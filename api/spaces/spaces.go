// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.spaces")

const spacesFacade = "Spaces"

// API provides access to the InstancePoller API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Spaces facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, spacesFacade)
	return &API{
		facade: facadeCaller,
	}
}

func makeCreateSpaceParams(name string, subnetIds []string, public bool) params.CreateSpaceParams {
	spaceTag := names.NewSpaceTag(name).String()
	subnetTags := make([]string, len(subnetIds))

	for i, s := range subnetIds {
		subnetTags[i] = names.NewSubnetTag(s).String()
	}

	return params.CreateSpaceParams{
		SpaceTag:   spaceTag,
		SubnetTags: subnetTags,
		Public:     public,
	}
}

// CreateSpace creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpace(name string, subnetIds []string, public bool) (params.ErrorResults, error) {
	var response params.ErrorResults
	params := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{makeCreateSpaceParams(name, subnetIds, public)},
	}
	err := api.facade.FacadeCall("CreateSpaces", params, &response)
	return response, err
}

// ListSpaces lists all available spaces and their associated subnets.
func (api *API) ListSpaces() ([]params.Space, error) {
	var response params.ListSpacesResults
	err := api.facade.FacadeCall("ListSpaces", nil, &response)
	return response.Results, err
}
