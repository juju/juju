// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("CrossModelRelations", 1, NewAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	access     crossmodelAccess
	authorizer common.Authorizer
}

// createAPI returns a new cross model API facade.
func createAPI(
	st crossmodelAccess,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		access:     st,
		authorizer: authorizer,
	}, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(getState(st), resources, authorizer)
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(offers params.CrossModelOffers) error {
	// TODO(anastasiamac 2015-11-02) validate:
	// service name valid and exists,
	// endpoints valid and exist,
	// url conforms to format,
	// users exist?
	return api.access.Offer(offers)
}
