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

func makeAddSpeceParams(name string, subnetIds []string, public bool) params.AddSpaceParams {
	spaceTag := names.NewSpaceTag(name).String()
	subnetTags := []string{}

	for _, s := range subnetIds {
		subnetTags = append(subnetTags, names.NewSubnetTag(s).String())
	}

	return params.AddSpaceParams{
		SpaceTag:   spaceTag,
		SubnetTags: subnetTags,
		Public:     public,
	}
}

// CreateSpace creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpace(name string, subnetIds []string, public bool) (params.ErrorResults, error) {
	var response params.ErrorResults
	params := params.AddSpacesParams{
		Spaces: []params.AddSpaceParams{makeAddSpeceParams(name, subnetIds, public)},
	}
	err := api.facade.FacadeCall("CreateSpaces", params, &response)
	return response, err
}
