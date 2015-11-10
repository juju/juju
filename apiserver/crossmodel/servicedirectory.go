// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/crossmodel"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("ServiceDirectory", 1, newServiceDirectoryAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type ServiceDirectoryAPI struct {
	authorizer       common.Authorizer
	serviceDirectory crossmodel.ServiceDirectoryProvider
}

// createServiceDirectoryAPI returns a new cross model API facade.
func createServiceDirectoryAPI(
	serviceDirectory crossmodel.ServiceDirectoryProvider,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ServiceDirectoryAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}

	return &ServiceDirectoryAPI{
		authorizer:       authorizer,
		serviceDirectory: serviceDirectory,
	}, nil
}

func newServiceDirectoryAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ServiceDirectoryAPI, error) {
	return createServiceDirectoryAPI(crossmodel.NewEmbeddedServiceDirectory(st), resources, authorizer)
}

// ListOffers returns offers matching the filter from a service directory.
func (api *ServiceDirectoryAPI) ListOffers(filters params.OfferFilters) ([]params.ServiceOffer, error) {
	return api.serviceDirectory.ListOffers(filters.Filters...)
}

// AddOffers adds new service offerings to a directory, able to be consumed by
// the specified users.
func (api *ServiceDirectoryAPI) AddOffers(offers params.AddServiceOffers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(offers.Offers)),
	}
	if len(offers.Offers) == 0 {
		return result, nil
	}

	for i, offer := range offers.Offers {
		if err := api.serviceDirectory.AddOffer(offer.ServiceURL, offer.ServiceOfferDetails, offer.Users); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}
