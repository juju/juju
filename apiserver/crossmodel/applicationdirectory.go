// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	crossmodelapi "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacadeForFeature("ApplicationOffers", 1, newApplicationOffersAPI, feature.CrossModelRelations)
}

// ApplicationOffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type ApplicationOffersAPI interface {
	// AddOffers adds new application offerings to a directory, able to be consumed by
	// the specified users.
	AddOffers(offers params.AddApplicationOffers) (params.ErrorResults, error)

	// ListOffers returns offers matching the filter from a application directory.
	ListOffers(filters params.OfferFilters) (params.ApplicationOfferResults, error)
}

type applicationOffersAPI struct {
	authorizer                  facade.Authorizer
	applicationOffersAPIFactory ApplicationOffersAPIFactory
	// TODO(wallyworld) - add component to handle permissions.
}

// createApplicationDirectoryAPI returns a new cross model API facade.
func createApplicationOffersAPI(
	serviceAPIFactory ApplicationOffersAPIFactory,
	authorizer facade.Authorizer,
) (ApplicationOffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &applicationOffersAPI{
		authorizer:                  authorizer,
		applicationOffersAPIFactory: serviceAPIFactory,
	}, nil
}

func newApplicationOffersAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (ApplicationOffersAPI, error) {
	apiFactory := resources.Get("applicationOffersApiFactory").(ApplicationOffersAPIFactory)
	return createApplicationOffersAPI(apiFactory, authorizer)
}

// TODO(wallyworld) - add Remove() and Update()

// ListOffers returns offers matching the filter from a application directory.
func (api *applicationOffersAPI) ListOffers(filters params.OfferFilters) (params.ApplicationOfferResults, error) {
	if filters.Directory == "" {
		return params.ApplicationOfferResults{}, errors.New("application directory must be specified")
	}
	applicationOffers, err := api.applicationOffersAPIFactory.ApplicationOffers(filters.Directory)
	if err != nil {
		return params.ApplicationOfferResults{}, err
	}
	return applicationOffers.ListOffers(filters)
}

// AddOffers adds new application offerings to a directory, able to be consumed by
// the specified users.
func (api *applicationOffersAPI) AddOffers(offers params.AddApplicationOffers) (params.ErrorResults, error) {
	if len(offers.Offers) == 0 {
		return params.ErrorResults{}, nil
	}

	// TODO(wallyworld) - we assume for now that all offers are to the
	// same backend ie all local or all to the same remote application directory.
	directory, err := jujucrossmodel.ApplicationDirectoryForURL(offers.Offers[0].ApplicationURL)
	if err != nil {
		return params.ErrorResults{}, err
	}
	applicationOffers, err := api.applicationOffersAPIFactory.ApplicationOffers(directory)
	if err != nil {
		return params.ErrorResults{}, err
	}
	return applicationOffers.AddOffers(offers)
}

// localApplicationOffers provides access to application offers hosted within
// a local controller.
type localApplicationOffers struct {
	applicationDirectory crossmodel.ApplicationDirectory
}

// ListOffers returns offers matching the filter from a application directory.
func (so *localApplicationOffers) ListOffers(filters params.OfferFilters) (params.ApplicationOfferResults, error) {
	var result params.ApplicationOfferResults
	offerFilters, err := makeOfferFilterFromParams(filters.Filters)
	if err != nil {
		return result, err
	}
	offers, err := so.applicationDirectory.ListOffers(offerFilters...)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	result.Offers = make([]params.ApplicationOffer, len(offers))
	for i, offer := range offers {
		result.Offers[i] = crossmodelapi.MakeParamsFromOffer(offer)
	}
	return result, nil
}

func makeOfferFilterFromParams(filters []params.OfferFilter) ([]crossmodel.ApplicationOfferFilter, error) {
	offerFilters := make([]crossmodel.ApplicationOfferFilter, len(filters))
	for i, filter := range filters {
		offerFilters[i] = crossmodel.ApplicationOfferFilter{
			ApplicationOffer: crossmodel.ApplicationOffer{
				ApplicationURL:         filter.ApplicationURL,
				ApplicationName:        filter.ApplicationName,
				ApplicationDescription: filter.ApplicationDescription,
				SourceLabel:            filter.SourceLabel,
			},
		}
		if filter.SourceModelUUIDTag != "" {
			envTag, err := names.ParseModelTag(filter.SourceModelUUIDTag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			offerFilters[i].SourceModelUUID = envTag.Id()
		}
		// TODO(wallyworld) - add support for Endpoint filter attribute
	}
	return offerFilters, nil
}

// AddOffers adds new application offerings to a directory, able to be consumed by
// the specified users.
func (so *localApplicationOffers) AddOffers(offers params.AddApplicationOffers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(offers.Offers)),
	}
	if len(offers.Offers) == 0 {
		return result, nil
	}

	for i, offerParams := range offers.Offers {
		offer, err := crossmodelapi.MakeOfferFromParams(offerParams.ApplicationOffer)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := so.applicationDirectory.AddOffer(offer); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	// TODO(wallyworld) - write ACLs with supplied users once we support that
	return result, nil
}
