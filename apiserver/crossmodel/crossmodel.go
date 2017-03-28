// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacadeForFeature("CrossModelRelations", 1, NewAPI, feature.CrossModelRelations)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer        facade.Authorizer
	applicationOffers jujucrossmodel.ApplicationOffers
	backend           Backend
}

// createAPI returns a new cross model API facade.
func createAPI(
	applicationOffers jujucrossmodel.ApplicationOffers,
	backend Backend,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	api := &API{
		authorizer:        authorizer,
		applicationOffers: applicationOffers,
		backend:           backend,
	}
	return api, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(
	st *state.State,
	_ facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	applicationOffers := state.NewApplicationOffers(st)
	return createAPI(applicationOffers, getStateAccess(st), authorizer)
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *API) Offer(all params.AddApplicationOffers) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))

	for i, one := range all.Offers {
		applicationOfferParams, err := api.makeAddOfferArgsFromParams(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		_, err = api.applicationOffers.AddOffer(applicationOfferParams)
		result[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *API) makeAddOfferArgsFromParams(addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		ApplicationURL:         addOfferParams.ApplicationURL,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
	}
	application, err := api.backend.Application(addOfferParams.ApplicationName)
	if err != nil {
		return result, errors.Annotatef(err, "getting offered application %v", addOfferParams.ApplicationName)
	}

	if result.ApplicationDescription == "" {
		ch, _, err := application.Charm()
		if err != nil {
			return result,
				errors.Annotatef(err, "getting charm for application %v", addOfferParams.ApplicationName)
		}
		result.ApplicationDescription = ch.Meta().Description
	}
	return result, nil
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *API) ApplicationOffers(urls params.ApplicationURLs) (params.ApplicationOffersResults, error) {
	var results params.ApplicationOffersResults

	foundOffers := make(map[string]jujucrossmodel.ApplicationOffer)
	filters := make([]jujucrossmodel.ApplicationOfferFilter, len(urls.ApplicationURLs))
	for i, url := range urls.ApplicationURLs {
		filters[i] = jujucrossmodel.ApplicationOfferFilter{ApplicationURL: url}
	}
	offers, err := api.applicationOffers.ListOffers(filters...)
	if err != nil {
		return results, errors.Trace(err)
	}
	for _, offer := range offers {
		foundOffers[offer.ApplicationURL] = offer
	}

	// We have the offers keyed on URL, sort out the not found URLs
	// from the supplied filter arguments.
	results.Results = make([]params.ApplicationOfferResult, len(urls.ApplicationURLs))
	for i, one := range urls.ApplicationURLs {
		foundOffer, ok := foundOffers[one]
		if !ok {
			results.Results[i].Error = common.ServerError(errors.NotFoundf("offer for remote application url %v", one))
			continue
		}
		results.Results[i].Result = makeOfferParamsFromOffer(foundOffer)
	}

	return results, nil
}

func makeOfferParamsFromOffer(offer jujucrossmodel.ApplicationOffer) params.ApplicationOffer {
	result := params.ApplicationOffer{
		ApplicationURL:         offer.ApplicationURL,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
	}
	for alias, ep := range offer.Endpoints {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      alias,
			Interface: ep.Interface,
			Role:      ep.Role,
			Scope:     ep.Scope,
			Limit:     ep.Limit,
		})
	}
	return result
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *API) FindApplicationOffers(filters params.OfferFilters) (params.FindApplicationOffersResults, error) {
	var result params.FindApplicationOffersResults

	offerFilters, err := makeOfferFilterFromParams(filters.Filters)
	if err != nil {
		return result, err
	}

	offers, err := api.applicationOffers.ListOffers(offerFilters...)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, offer := range offers {
		result.Results = append(result.Results, makeOfferParamsFromOffer(offer))
	}
	return result, nil
}

func makeOfferFilterFromParams(filters []params.OfferFilter) ([]jujucrossmodel.ApplicationOfferFilter, error) {
	offerFilters := make([]jujucrossmodel.ApplicationOfferFilter, len(filters))
	for i, filter := range filters {
		offerFilters[i] = jujucrossmodel.ApplicationOfferFilter{
			ApplicationURL:         filter.ApplicationURL,
			ApplicationName:        filter.ApplicationName,
			ApplicationDescription: filter.ApplicationDescription,
		}
		// TODO(wallyworld) - add support for Endpoint filter attribute
	}
	return offerFilters, nil
}
