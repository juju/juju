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
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.crossmodel")

func init() {
	common.RegisterStandardFacade("CrossModelRelations", 1, NewAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer common.Authorizer
	backend    ServicesBackend
	access     stateAccess
}

// createAPI returns a new cross model API facade.
func createAPI(
	backend ServicesBackend,
	access stateAccess,
	resources *common.Resources,
	authorizer common.Authorizer,
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
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	serviceOffers, err := newServiceOffersAPI(st, resources, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offeredServices := state.NewOfferedServices(st)
	backend := &servicesBackend{
		offeredServices,
		serviceOffers,
	}
	return createAPI(backend, getStateAccess(st), resources, authorizer)
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(all params.ServiceOffersParams) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))
	for i, one := range all.Offers {
		offer, serviceOfferParams, err := api.makeOfferedServiceParams(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		if err := api.backend.AddOffer(offer, params.AddServiceOffer{
			ServiceOffer: serviceOfferParams,
			UserTags:     one.AllowedUserTags,
		}); err != nil {
			result[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: result}, nil
}

// makeOfferedServiceParams is a helper function that translates from a params
// structure into data structures needed for subsequent processing.
func (api *API) makeOfferedServiceParams(p params.ServiceOfferParams) (jujucrossmodel.OfferedService, params.ServiceOffer, error) {
	service, err := api.access.Service(p.ServiceName)
	if err != nil {
		return jujucrossmodel.OfferedService{}, params.ServiceOffer{}, errors.Annotatef(err, "getting offered service %v", p.ServiceName)
	}

	endpoints, err := getServiceEndpoints(service, p.Endpoints)
	if err != nil {
		return jujucrossmodel.OfferedService{}, params.ServiceOffer{}, errors.Trace(err)
	}
	curl, _ := service.CharmURL()
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

	// offer is used to record the offered service in the host environment.
	offer := jujucrossmodel.OfferedService{
		ServiceURL:  p.ServiceURL,
		ServiceName: service.Name(),
		CharmName:   curl.Name,
		Endpoints:   epNames,
		Description: p.ServiceDescription,
	}

	if offer.Description == "" || offer.Icon == nil {
		ch, _, err := service.Charm()
		if err != nil {
			return jujucrossmodel.OfferedService{}, params.ServiceOffer{}, errors.Annotatef(err, "getting charm for service %v", p.ServiceName)
		}
		if offer.Description == "" {
			offer.Description = ch.Meta().Description
		}
		if offer.Icon == nil {
			// TODO(wallyworld) - add charm icon.
		}
	}

	// TODO(wallyworld) - allow source env name to be aliased
	envName, err := api.access.EnvironName()
	if err != nil {
		return jujucrossmodel.OfferedService{}, params.ServiceOffer{}, errors.Trace(err)
	}

	// offerParams is used to make the API call to record the service offer
	// in a service directory.
	offerParams := params.ServiceOffer{
		ServiceURL:         offer.ServiceURL,
		ServiceName:        offer.ServiceName,
		ServiceDescription: offer.Description,
		Endpoints:          remoteEndpoints,
		SourceEnvironTag:   api.access.EnvironTag().String(),
		SourceLabel:        envName,
	}

	return offer, offerParams, nil
}

// ServiceOffers gets details about remote services that match given URLs.
func (api *API) ServiceOffers(filter params.ServiceURLs) (params.ServiceOffersResults, error) {
	urls := filter.ServiceUrls
	results := make([]params.ServiceOfferResult, len(urls))
	// Record errors for each URL for later.
	errorsByURL := make(map[string]error)

	// Group the filter URL terms by directory name so that the
	// service directory API for each named directory can be used
	// via a bulk call.
	urlsByDirectory := make(map[string][]string)
	for _, urlstr := range filter.ServiceUrls {
		url, err := jujucrossmodel.ParseServiceURL(urlstr)
		if err != nil {
			errorsByURL[urlstr] = err
			continue
		}
		urlsByDirectory[url.Directory] = append(urlsByDirectory[url.Directory], urlstr)
	}

	foundOffers := make(map[string]params.ServiceOffer)
	for directory, urls := range urlsByDirectory {
		// Make the filter terms for the current directory and then
		// list the offers for that directory.
		filters := params.OfferFilters{Directory: directory}
		filters.Filters = make([]params.OfferFilter, len(urls))
		for i, url := range urls {
			filters.Filters[i] = params.OfferFilter{ServiceURL: url}
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
			foundOffers[offer.ServiceURL] = offer
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
			results[i].Error = common.ServerError(errors.NotFoundf("offer for remote service url %v", one))
			continue
		}
		results[i].Result = foundOffer
	}
	return params.ServiceOffersResults{results}, nil
}

// ListOffers gets all remote services that have been offered from this Juju model.
// Each returned service satisfies at least one of the the specified filters.
func (api *API) ListOffers(args params.OfferedServiceFilters) (params.ListOffersResults, error) {

	// This func constructs individual set of filters.
	filters := func(aSet params.OfferedServiceFilter) []jujucrossmodel.OfferedServiceFilter {
		result := make([]jujucrossmodel.OfferedServiceFilter, len(aSet.FilterTerms))
		for i, filter := range aSet.FilterTerms {
			result[i] = constructOfferedServiceFilter(filter)
		}
		return result
	}

	// This func converts results for a filters set to params.
	convertToParams := func(offered []jujucrossmodel.OfferedService) []params.OfferedServiceDetailsResult {
		results := make([]params.OfferedServiceDetailsResult, len(offered))
		for i, one := range offered {
			results[i] = api.getOfferedService(one)
		}
		return results
	}

	found := make([]params.ListOffersFilterResults, len(args.Filters))
	for i, set := range args.Filters {
		setResult, err := api.backend.ListOfferedServices(filters(set)...)
		if err != nil {
			found[i].Error = common.ServerError(err)
			continue
		}
		found[i].Result = convertToParams(setResult)
	}

	return params.ListOffersResults{found}, nil
}

func (api *API) getOfferedService(remote jujucrossmodel.OfferedService) params.OfferedServiceDetailsResult {
	service, err := api.access.Service(remote.ServiceName)
	if err != nil {
		return params.OfferedServiceDetailsResult{Error: common.ServerError(err)}
	}

	ch, _, err := service.Charm()
	if err != nil {
		return params.OfferedServiceDetailsResult{Error: common.ServerError(err)}
	}
	var epNames []string
	for name, _ := range remote.Endpoints {
		epNames = append(epNames, name)
	}
	charmEps, err := getServiceEndpoints(service, epNames)
	if err != nil {
		return params.OfferedServiceDetailsResult{Error: common.ServerError(err)}
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
	result := params.OfferedServiceDetails{
		Endpoints:   eps,
		CharmName:   ch.Meta().Name,
		ServiceName: remote.ServiceName,
		ServiceURL:  remote.ServiceURL,
		// TODO (wallyworld) - find connected users count
		//UsersCount: 0,
	}
	return params.OfferedServiceDetailsResult{Result: &result}
}

func constructOfferedServiceFilter(filter params.OfferedServiceFilterTerm) jujucrossmodel.OfferedServiceFilter {
	return jujucrossmodel.OfferedServiceFilter{
		ServiceURL: filter.ServiceURL,
		CharmName:  filter.CharmName,
		// TODO (wallyworld) - support filtering offered service by endpoint
		//		Endpoint: jujucrossmodel.EndpointFilterTerm{
		//			Name:      filter.Endpoint.Name,
		//			Interface: filter.Endpoint.Interface,
		//			Role:      filter.Endpoint.Role,
		//		},
	}
}

func getServiceEndpoints(service *state.Service, endpointNames []string) ([]charm.Relation, error) {
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

	// AddOffer adds a new service offer to the directory.
	AddOffer(offer jujucrossmodel.OfferedService, offerParams params.AddServiceOffer) error

	// ListOfferedServices returns offered services satisfying specified filters.
	ListOfferedServices(filter ...jujucrossmodel.OfferedServiceFilter) ([]jujucrossmodel.OfferedService, error)

	// ListDirectoryOffers returns service directory offers satisfying the specified filter.
	ListDirectoryOffers(filter params.OfferFilters) (params.ServiceOfferResults, error)
}

var _ ServicesBackend = (*servicesBackend)(nil)

type servicesBackend struct {
	offeredServices jujucrossmodel.OfferedServices
	serviceOffers   ServiceOffersAPI
}

func (s *servicesBackend) AddOffer(offer jujucrossmodel.OfferedService, offerParams params.AddServiceOffer) error {
	// Add the offer to the offered services collection for the host environment.
	err := s.offeredServices.AddOffer(offer)
	if err != nil {
		return errors.Trace(err)
	}

	// Record the offer in a directory of service offers.
	errResult, err := s.serviceOffers.AddOffers(params.AddServiceOffers{
		Offers: []params.AddServiceOffer{offerParams},
	})
	if err != nil {
		return err
	}
	return errResult.OneError()
}

func (s *servicesBackend) ListOfferedServices(filter ...jujucrossmodel.OfferedServiceFilter) ([]jujucrossmodel.OfferedService, error) {
	return s.offeredServices.ListOffers(filter...)
}

func (s *servicesBackend) ListDirectoryOffers(filter params.OfferFilters) (params.ServiceOfferResults, error) {
	return s.serviceOffers.ListOffers(filter)
}
