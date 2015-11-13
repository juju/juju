// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	crossmodelapi "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

func init() {
	common.RegisterStandardFacade("ServiceDirectory", 1, newServiceDirectoryAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type ServiceDirectoryAPI struct {
	authorizer       common.Authorizer
	serviceDirectory crossmodel.ServiceDirectory
}

// createServiceDirectoryAPI returns a new cross model API facade.
func createServiceDirectoryAPI(
	serviceDirectory crossmodel.ServiceDirectory,
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
	return createServiceDirectoryAPI(state.NewServiceDirectory(st), resources, authorizer)
}

// TODO(wallyworld) - add Remove() and Update()

// ListOffers returns offers matching the filter from a service directory.
func (api *ServiceDirectoryAPI) ListOffers(filters params.OfferFilters) (params.ServiceOfferResults, error) {
	var result params.ServiceOfferResults
	offerFilters, err := makeOfferFilterFromParams(filters.Filters)
	if err != nil {
		return result, err
	}
	offers, err := api.serviceDirectory.ListOffers(offerFilters...)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	result.Offers = make([]params.ServiceOffer, len(offers))
	for i, offer := range offers {
		result.Offers[i] = crossmodelapi.MakeParamsFromOffer(offer)
	}
	return result, nil
}

func makeOfferFilterFromParams(filters []params.OfferFilter) ([]crossmodel.ServiceOfferFilter, error) {
	offerFilters := make([]crossmodel.ServiceOfferFilter, len(filters))
	for i, filter := range filters {
		offerFilters[i] = crossmodel.ServiceOfferFilter{
			ServiceOffer: crossmodel.ServiceOffer{
				ServiceURL:         filter.ServiceURL,
				ServiceName:        filter.ServiceName,
				ServiceDescription: filter.ServiceDescription,
				SourceLabel:        filter.SourceLabel,
			},
		}
		if filter.SourceEnvUUIDTag != "" {
			envTag, err := names.ParseEnvironTag(filter.SourceEnvUUIDTag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			offerFilters[i].SourceEnvUUID = envTag.Id()
		}
	}
	return offerFilters, nil
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

	for i, offerParams := range offers.Offers {
		offer, err := crossmodelapi.MakeOfferFromParams(offerParams.ServiceOffer)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.serviceDirectory.AddOffer(offer); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	// TODO(wallyworld) - write ACLs with supplied users once we support that
	return result, nil
}
