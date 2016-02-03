// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.api.discoverspaces")

const discoverspacesFacade = "DiscoverSpaces"

// API provides access to the DiscoverSpaces API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, discoverspacesFacade)
	return &API{
		facade: facadeCaller,
	}
}

func (api *API) ListSpaces() (params.DiscoverSpacesResults, error) {
	var result params.DiscoverSpacesResults
	if err := api.facade.FacadeCall("ListSpaces", nil, &result); err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func (api *API) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	var result params.ErrorResults
	err := api.facade.FacadeCall("AddSubnets", args, &result)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func (api *API) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	var result params.ErrorResults
	err = api.facade.FacadeCall("CreateSpaces", args, &result)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func (api *API) ListSubnets(args params.SubnetsFilters) (params.ListSubnetsResults, error) {
	var result params.ListSubnetsResults
	if err := api.facade.FacadeCall("ListSubnets", args, &result); err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

// ModelConfig returns the current model configuration.
func (api *API) ModelConfig() (*config.Config, error) {
	var result params.ModelConfigResult
	err := api.facade.FacadeCall("ModelConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}
