// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	crossmodelapi "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

func init() {
	common.RegisterStandardFacade("ServiceOffers", 1, newServiceOffersAPI)
}

// ServiceOffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type ServiceOffersAPI interface {
	// AddOffers adds new service offerings to a directory, able to be consumed by
	// the specified users.
	AddOffers(offers params.AddServiceOffers) (params.ErrorResults, error)

	// ListOffers returns offers matching the filter from a service directory.
	ListOffers(filters params.OfferFilters) (params.ServiceOfferResults, error)
}

type serviceOffersAPI struct {
	authorizer              common.Authorizer
	serviceOffersAPIFactory ServiceOffersAPIFactory
	// TODO(wallyworld) - add component to handle permissions.
}

// createServiceDirectoryAPI returns a new cross model API facade.
func createServiceOffersAPI(
	serviceAPIFactory ServiceOffersAPIFactory,
	resources *common.Resources,
	authorizer common.Authorizer,
) (ServiceOffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &serviceOffersAPI{
		authorizer:              authorizer,
		serviceOffersAPIFactory: serviceAPIFactory,
	}, nil
}

func newServiceOffersAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (ServiceOffersAPI, error) {
	apiFactory, err := newServiceAPIFactory(func() crossmodel.ServiceDirectory {
		return state.NewServiceDirectory(st)
	})
	if err != nil {
		return nil, err
	}
	return createServiceOffersAPI(apiFactory, resources, authorizer)
}

// TODO(wallyworld) - add Remove() and Update()

// ListOffers returns offers matching the filter from a service directory.
func (api *serviceOffersAPI) ListOffers(filters params.OfferFilters) (params.ServiceOfferResults, error) {
	if filters.Directory == "" {
		return params.ServiceOfferResults{}, errors.New("service directory must be specified")
	}
	serviceOffers, err := api.serviceOffersAPIFactory.ServiceOffers(filters.Directory)
	if err != nil {
		return params.ServiceOfferResults{}, err
	}
	return serviceOffers.ListOffers(filters)
}

// AddOffers adds new service offerings to a directory, able to be consumed by
// the specified users.
func (api *serviceOffersAPI) AddOffers(offers params.AddServiceOffers) (params.ErrorResults, error) {
	if len(offers.Offers) == 0 {
		return params.ErrorResults{}, nil
	}

	// TODO(wallyworld) - we assume for now that all offers are to the
	// same backend ie all local or all to the same remote service directory.
	directory, err := jujucrossmodel.ServiceDirectoryForURL(offers.Offers[0].ServiceURL)
	if err != nil {
		return params.ErrorResults{}, err
	}
	serviceOffers, err := api.serviceOffersAPIFactory.ServiceOffers(directory)
	if err != nil {
		return params.ErrorResults{}, err
	}
	return serviceOffers.AddOffers(offers)
}

// localServiceOffers provides access to service offers hosted within
// a local controller.
type localServiceOffers struct {
	serviceDirectory crossmodel.ServiceDirectory
}

// ListOffers returns offers matching the filter from a service directory.
func (so *localServiceOffers) ListOffers(filters params.OfferFilters) (params.ServiceOfferResults, error) {
	var result params.ServiceOfferResults
	offerFilters, err := makeOfferFilterFromParams(filters.Filters)
	if err != nil {
		return result, err
	}
	offers, err := so.serviceDirectory.ListOffers(offerFilters...)
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
func (so *localServiceOffers) AddOffers(offers params.AddServiceOffers) (params.ErrorResults, error) {
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
		if err := so.serviceDirectory.AddOffer(offer); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	// TODO(wallyworld) - write ACLs with supplied users once we support that
	return result, nil
}
