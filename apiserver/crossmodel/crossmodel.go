// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.crossmodel")

func init() {
	common.RegisterStandardFacadeForFeature("CrossModelRelations", 1, NewAPI, feature.CrossModelRelations)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer                       facade.Authorizer
	applicationDirectory             ApplicationOffersAPI
	backend                          Backend
	makeOfferedApplicationParamsFunc func(p params.ApplicationOfferParams) (params.ApplicationOffer, error)
}

// createAPI returns a new cross model API facade.
func createAPI(
	applicationDirectory ApplicationOffersAPI,
	backend Backend,
	authorizer facade.Authorizer,
	makeOfferedApplicationParamsFunc func(p params.ApplicationOfferParams) (params.ApplicationOffer, error),
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	api := &API{
		authorizer:           authorizer,
		applicationDirectory: applicationDirectory,
		backend:              backend,
		makeOfferedApplicationParamsFunc: makeOfferedApplicationParamsFunc,
	}
	if makeOfferedApplicationParamsFunc == nil {
		api.makeOfferedApplicationParamsFunc = api.makeOfferedApplicationParams
	}
	return api, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	applicationOffers, err := newApplicationOffersAPI(st, resources, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return createAPI(applicationOffers, getStateAccess(st), authorizer, nil)
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *API) Offer(all params.ApplicationOffersParams) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))

	var offers params.AddApplicationOffers
	indexInOffersToResult := make(map[int]int)
	for i, one := range all.Offers {
		applicationOfferParams, err := api.makeOfferedApplicationParamsFunc(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		indexInOffersToResult[len(offers.Offers)] = i
		offers.Offers = append(offers.Offers, params.AddApplicationOffer{
			ApplicationOffer: applicationOfferParams,
			UserTags:         one.AllowedUserTags,
		})
	}
	addOffersErrors, err := api.applicationDirectory.AddOffers(offers)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	// Merge the results with any errors encountered earlier.
	for i, errResult := range addOffersErrors.Results {
		if resultIndex, ok := indexInOffersToResult[i]; ok {
			result[resultIndex] = errResult
		}
	}
	return params.ErrorResults{Results: result}, nil
}

// makeOfferedApplicationParams is a helper function that translates from a params
// structure into data structures needed for subsequent processing.
func (api *API) makeOfferedApplicationParams(p params.ApplicationOfferParams) (params.ApplicationOffer, error) {
	tag, err := names.ParseModelTag(p.ModelTag)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}
	st, err := api.backend.ForModel(tag)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}
	defer st.Close()
	application, err := st.Application(p.ApplicationName)
	if err != nil {
		return params.ApplicationOffer{}, errors.Annotatef(err, "getting offered application %v", p.ApplicationName)
	}

	endpoints, err := getApplicationEndpoints(application, p.Endpoints)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}
	curl, _ := application.CharmURL()
	epNames := make(map[string]string)
	remoteEndpoints := make([]params.RemoteEndpoint, len(endpoints))
	for i, ep := range endpoints {
		// TODO(wallyworld) - allow endpoint name aliasing
		epNames[ep.Name] = ep.Name
		remoteEndpoints[i] = params.RemoteEndpoint{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}

	offer := jujucrossmodel.OfferedApplication{
		ApplicationURL:  p.ApplicationURL,
		ApplicationName: application.Name(),
		CharmName:       curl.Name,
		Endpoints:       epNames,
		Description:     p.ApplicationDescription,
	}

	if offer.Description == "" || offer.Icon == nil {
		ch, _, err := application.Charm()
		if err != nil {
			return params.ApplicationOffer{},
				errors.Annotatef(err, "getting charm for application %v", p.ApplicationName)
		}
		if offer.Description == "" {
			offer.Description = ch.Meta().Description
		}
		if offer.Icon == nil {
			// TODO(wallyworld) - add charm icon.
		}
	}

	// TODO(wallyworld) - allow source model name to be aliased
	sourceModel, err := st.Model()
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}

	// offerParams is used to make the API call to record the application offer
	// in a application directory.
	offerParams := params.ApplicationOffer{
		ApplicationURL:         offer.ApplicationURL,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.Description,
		Endpoints:              remoteEndpoints,
		SourceModelTag:         st.ModelTag().String(),
		SourceLabel:            sourceModel.Name(),
	}

	return offerParams, nil
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *API) ApplicationOffers(filter params.ApplicationURLs) (params.ApplicationOffersResults, error) {
	urls := filter.ApplicationURLs
	results := make([]params.ApplicationOfferResult, len(urls))
	// Record errors for each URL for later.
	errorsByURL := make(map[string]error)

	// Group the filter URL terms by directory name so that the
	// application directory API for each named directory can be used
	// via a bulk call.
	urlsByDirectory := make(map[string][]string)
	for _, urlstr := range filter.ApplicationURLs {
		url, err := jujucrossmodel.ParseApplicationURL(urlstr)
		if err != nil {
			errorsByURL[urlstr] = err
			continue
		}
		urlsByDirectory[url.Directory] = append(urlsByDirectory[url.Directory], urlstr)
	}

	foundOffers := make(map[string]params.ApplicationOffer)
	for directory, urls := range urlsByDirectory {
		// Make the filter terms for the current directory and then
		// list the offers for that directory.
		filters := params.OfferFilters{Directory: directory}
		filters.Filters = make([]params.OfferFilter, len(urls))
		for i, url := range urls {
			filters.Filters[i] = params.OfferFilter{ApplicationURL: url}
		}
		offers, err := api.applicationDirectory.ListOffers(filters)
		if err == nil && offers.Error != nil {
			err = offers.Error
		}
		if err != nil {
			for _, url := range urls {
				errorsByURL[url] = err
			}
			continue
		}
		for _, offer := range offers.Offers {
			foundOffers[offer.ApplicationURL] = offer
		}
	}

	// We have the offers keyed on URL, sort out the not found URLs
	// from the supplied filter arguments.
	for i, one := range urls {
		if err, ok := errorsByURL[one]; ok {
			results[i].Error = common.ServerError(err)
			continue
		}
		foundOffer, ok := foundOffers[one]
		if !ok {
			results[i].Error = common.ServerError(errors.NotFoundf("offer for remote application url %v", one))
			continue
		}
		results[i].Result = foundOffer
	}
	return params.ApplicationOffersResults{results}, nil
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *API) FindApplicationOffers(filters params.OfferFilterParams) (params.FindApplicationOffersResults, error) {
	var result params.FindApplicationOffersResults
	result.Results = make([]params.ApplicationOfferResults, len(filters.Filters))

	for i, filter := range filters.Filters {
		offers, err := api.applicationDirectory.ListOffers(filter)
		if err == nil && offers.Error != nil {
			err = offers.Error
		}
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i] = offers
	}
	return result, nil
}

func getApplicationEndpoints(application *state.Application, endpointNames []string) ([]charm.Relation, error) {
	result := make([]charm.Relation, len(endpointNames))
	for i, endpointName := range endpointNames {
		endpoint, err := application.Endpoint(endpointName)
		if err != nil {
			return nil, errors.Annotatef(err, "getting relation endpoint for relation %q and application %q", endpointName, application.Name())
		}
		result[i] = endpoint.Relation
	}
	return result, nil
}
