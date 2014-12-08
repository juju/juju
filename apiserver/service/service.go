// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service contains api calls for accessing service functionality.
package service

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.service")

func init() {
	common.RegisterStandardFacade("Service", 0, NewServiceAPI)
}

// Service defines the methods on the service API end point.
type Service interface {
	SetMetricCredentials(args params.ServiceMetricCredentials) (params.ErrorResults, error)
}

// API implements the service interface and is the concrete
// implementation of the api end point.
type API struct {
	state      *state.State
	authorizer common.Authorizer
}

var _ Service = (*API)(nil)

// NewAPI returns a new service API facade.
func NewServiceAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		state:      st,
		authorizer: authorizer,
	}, nil
}

// SetMetricCredentials sets credentials on the service.
func (api *API) SetMetricCredentials(args params.ServiceMetricCredentials) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Creds)),
	}
	if len(args.Creds) == 0 {
		return result, nil
	}
	for i, a := range args.Creds {
		service, err := api.state.Service(a.ServiceName)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = service.SetMetricCredentials(a.MetricCredentials)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}
