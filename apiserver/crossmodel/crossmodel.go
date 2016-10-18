// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.crossmodel")

func init() {
	common.RegisterStandardFacade("CrossModelRelations", 1, NewAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer facade.Authorizer
	backend    ServicesBackend
	access     stateAccess
}

// createAPI returns a new cross model API facade.
func createAPI(
	backend ServicesBackend,
	access stateAccess,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		authorizer: authorizer,
		backend:    backend,
		access:     access,
	}, nil
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
	offeredApplications := state.NewOfferedApplications(st)
	backend := &servicesBackend{
		offeredApplications,
		applicationOffers,
	}
	return createAPI(backend, getStateAccess(st), resources, authorizer)
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(all params.ApplicationOffersParams) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))
	for i, one := range all.Offers {
		offer, applicationOfferParams, err := api.makeOfferedApplicationParams(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		if err := api.backend.AddOffer(offer, params.AddApplicationOffer{
			ApplicationOffer: applicationOfferParams,
			UserTags:         one.AllowedUserTags,
		}); err != nil {
			result[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: result}, nil
}

// makeOfferedApplicationParams is a helper function that translates from a params
// structure into data structures needed for subsequent processing.
func (api *API) makeOfferedApplicationParams(p params.ApplicationOfferParams) (jujucrossmodel.OfferedApplication, params.ApplicationOffer, error) {
	application, err := api.access.Application(p.ApplicationName)
	if err != nil {
		return jujucrossmodel.OfferedApplication{}, params.ApplicationOffer{}, errors.Annotatef(err, "getting offered application %v", p.ApplicationName)
	}

	endpoints, err := getApplicationEndpoints(application, p.Endpoints)
	if err != nil {
		return jujucrossmodel.OfferedApplication{}, params.ApplicationOffer{}, errors.Trace(err)
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

	// offer is used to record the offered application in the host environment.
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
			return jujucrossmodel.OfferedApplication{}, params.ApplicationOffer{}, errors.Annotatef(err, "getting charm for service %v", p.ApplicationName)
		}
		if offer.Description == "" {
			offer.Description = ch.Meta().Description
		}
		if offer.Icon == nil {
			// TODO(wallyworld) - add charm icon.
		}
	}

	// TODO(wallyworld) - allow source model name to be aliased
	modelName, err := api.access.ModelName()
	if err != nil {
		return jujucrossmodel.OfferedApplication{}, params.ApplicationOffer{}, errors.Trace(err)
	}

	// offerParams is used to make the API call to record the application offer
	// in a application directory.
	offerParams := params.ApplicationOffer{
		ApplicationURL:         offer.ApplicationURL,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.Description,
		Endpoints:              remoteEndpoints,
		SourceModelTag:         api.access.ModelTag().String(),
		SourceLabel:            modelName,
	}

	return offer, offerParams, nil
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
		offers, err := api.backend.ListDirectoryOffers(filters)
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
		offers, err := api.backend.ListDirectoryOffers(filter)
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

// ListOffers gets all remote applications that have been offered from this Juju model.
// Each returned service satisfies at least one of the the specified filters.
func (api *API) ListOffers(args params.OfferedApplicationFilters) (params.ListOffersResults, error) {

	// This func constructs individual set of filters.
	filters := func(aSet params.OfferedApplicationFilter) []jujucrossmodel.OfferedApplicationFilter {
		result := make([]jujucrossmodel.OfferedApplicationFilter, len(aSet.FilterTerms))
		for i, filter := range aSet.FilterTerms {
			result[i] = constructOfferedApplicationFilter(filter)
		}
		return result
	}

	// This func converts results for a filters set to params.
	convertToParams := func(offered []jujucrossmodel.OfferedApplication) []params.OfferedApplicationDetailsResult {
		results := make([]params.OfferedApplicationDetailsResult, len(offered))
		for i, one := range offered {
			results[i] = api.getOfferedApplication(one)
		}
		return results
	}

	found := make([]params.ListOffersFilterResults, len(args.Filters))
	for i, set := range args.Filters {
		setResult, err := api.backend.ListOfferedApplications(filters(set)...)
		if err != nil {
			found[i].Error = common.ServerError(err)
			continue
		}
		found[i].Result = convertToParams(setResult)
	}

	return params.ListOffersResults{found}, nil
}

func (api *API) getOfferedApplication(remote jujucrossmodel.OfferedApplication) params.OfferedApplicationDetailsResult {
	application, err := api.access.Application(remote.ApplicationName)
	if err != nil {
		return params.OfferedApplicationDetailsResult{Error: common.ServerError(err)}
	}

	ch, _, err := application.Charm()
	if err != nil {
		return params.OfferedApplicationDetailsResult{Error: common.ServerError(err)}
	}
	var epNames []string
	for name, _ := range remote.Endpoints {
		epNames = append(epNames, name)
	}
	charmEps, err := getApplicationEndpoints(application, epNames)
	if err != nil {
		return params.OfferedApplicationDetailsResult{Error: common.ServerError(err)}
	}

	eps := make([]params.RemoteEndpoint, len(charmEps))
	for i, ep := range charmEps {
		eps[i] = params.RemoteEndpoint{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	result := params.OfferedApplicationDetails{
		Endpoints:       eps,
		CharmName:       ch.Meta().Name,
		ApplicationName: remote.ApplicationName,
		ApplicationURL:  remote.ApplicationURL,
		// TODO (wallyworld) - find connected users count
		//UsersCount: 0,
	}
	return params.OfferedApplicationDetailsResult{Result: &result}
}

func constructOfferedApplicationFilter(filter params.OfferedApplicationFilterTerm) jujucrossmodel.OfferedApplicationFilter {
	return jujucrossmodel.OfferedApplicationFilter{
		ApplicationURL: filter.ApplicationURL,
		CharmName:      filter.CharmName,
		// TODO (wallyworld) - support filtering offered application by endpoint
		//		Endpoint: jujucrossmodel.EndpointFilterTerm{
		//			Name:      filter.Endpoint.Name,
		//			Interface: filter.Endpoint.Interface,
		//			Role:      filter.Endpoint.Role,
		//		},
	}
}

func getApplicationEndpoints(service *state.Application, endpointNames []string) ([]charm.Relation, error) {
	result := make([]charm.Relation, len(endpointNames))
	for i, endpointName := range endpointNames {
		endpoint, err := service.Endpoint(endpointName)
		if err != nil {
			return nil, errors.Annotatef(err, "getting relation endpoint for relation %v and service %v", endpointName, service.Name())
		}
		result[i] = endpoint.Relation
	}
	return result, nil
}

// A ServicesBackend holds interface that this api requires.
type ServicesBackend interface {

	// AddOffer adds a new application offer to the directory.
	AddOffer(offer jujucrossmodel.OfferedApplication, offerParams params.AddApplicationOffer) error

	// ListOfferedApplications returns offered applications satisfying specified filters.
	ListOfferedApplications(filter ...jujucrossmodel.OfferedApplicationFilter) ([]jujucrossmodel.OfferedApplication, error)

	// ListDirectoryOffers returns application directory offers satisfying the specified filter.
	ListDirectoryOffers(filter params.OfferFilters) (params.ApplicationOfferResults, error)
}

var _ ServicesBackend = (*servicesBackend)(nil)

type servicesBackend struct {
	offeredApplications jujucrossmodel.OfferedApplications
	applicationOffers   ApplicationOffersAPI
}

func (s *servicesBackend) AddOffer(offer jujucrossmodel.OfferedApplication, offerParams params.AddApplicationOffer) error {
	// Add the offer to the offered applications collection for the host environment.
	err := s.offeredApplications.AddOffer(offer)
	if err != nil {
		return errors.Trace(err)
	}

	// Record the offer in a directory of application offers.
	errResult, err := s.applicationOffers.AddOffers(params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{offerParams},
	})
	if err != nil {
		return err
	}
	return errResult.OneError()
}

func (s *servicesBackend) ListOfferedApplications(filter ...jujucrossmodel.OfferedApplicationFilter) ([]jujucrossmodel.OfferedApplication, error) {
	return s.offeredApplications.ListOffers(filter...)
}

func (s *servicesBackend) ListDirectoryOffers(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
	return s.applicationOffers.ListOffers(filter)
}
