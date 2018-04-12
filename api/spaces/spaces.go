// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const spacesFacade = "Spaces"

// API provides access to the InstancePoller API facade.
type API struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Spaces facade.
func NewAPI(caller base.APICallCloser) *API {
	if caller == nil {
		panic("caller is nil")
	}
	clientFacade, facadeCaller := base.NewClientFacade(caller, spacesFacade)
	return &API{
		ClientFacade: clientFacade,
		facade:       facadeCaller,
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
func (api *API) CreateSpace(name string, subnetIds []string, public bool) error {
	var response params.ErrorResults
	createSpacesParams := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{makeCreateSpaceParams(name, subnetIds, public)},
	}
	err := api.facade.FacadeCall("CreateSpaces", createSpacesParams, &response)
	if err != nil {
		if params.IsCodeNotSupported(err) {
			return errors.NewNotSupported(nil, err.Error())
		}
		return errors.Trace(err)
	}
	return response.OneError()
}

// ListSpaces lists all available spaces and their associated subnets.
func (api *API) ListSpaces() ([]params.Space, error) {
	var response params.ListSpacesResults
	err := api.facade.FacadeCall("ListSpaces", nil, &response)
	if params.IsCodeNotSupported(err) {
		return response.Results, errors.NewNotSupported(nil, err.Error())
	}
	return response.Results, err
}

// ReloadSpaces reloads spaces from substrate
func (api *API) ReloadSpaces() error {
	if api.facade.BestAPIVersion() < 3 {
		return errors.NewNotSupported(nil, "Controller does not support reloading spaces")
	}
	err := api.facade.FacadeCall("ReloadSpaces", nil, nil)
	if params.IsCodeNotSupported(err) {
		return errors.NewNotSupported(nil, err.Error())
	}
	return err
}
