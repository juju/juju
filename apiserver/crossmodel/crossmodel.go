// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	model "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

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
	return createAPI(&servicesBackendStub{}, getStateAccess(st), resources, authorizer)
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(all params.RemoteServiceOffers) (params.ErrorResults, error) {
	cfg, err := api.access.EnvironConfig()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	offers := make([]params.ErrorResult, len(all.Offers))
	for i, one := range all.Offers {
		offer, err := api.parseOffer(one)
		if err != nil {
			offers[i].Error = common.ServerError(err)
			continue
		}

		offer.SourceLabel = cfg.Name()
		offer.SourceEnvUUID = api.access.EnvironUUID()

		if err := api.backend.AddOffer(offer); err != nil {
			offers[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: offers}, nil
}

// Show gets details about remote services that match given URLs.
func (api *API) Show(filter params.ShowFilter) (params.RemoteServiceResults, error) {
	urls := filter.URLs
	results := make([]params.RemoteServiceResult, len(urls))

	filters := make([]model.ServiceOfferFilter, len(urls))
	for i, one := range urls {
		if _, err := model.ParseServiceURL(one); err != nil {
			results[i].Error = common.ServerError(err)
		}
		filters[i].ServiceURL = one
	}

	found, err := api.backend.ListOffers(filters...)
	if err != nil {
		return params.RemoteServiceResults{}, errors.Trace(err)
	}

	tpMap := make(map[string]model.ServiceOffer, len(found))
	for _, offer := range found {
		tpMap[offer.ServiceURL] = offer
	}

	for i, one := range urls {
		foundOffer, ok := tpMap[one]
		if !ok {
			if results[i].Error != nil {
				// This means that url was invalid and the error was inserted above
				continue
			}
			results[i].Error = common.ServerError(errors.NotFoundf("offer for remote service url %v", one))
			continue
		}
		results[i].Result = convertServiceOffer(foundOffer)
	}
	return params.RemoteServiceResults{results}, nil
}

// List gets all remote services that have been offered from this Juju model.
// Returned list satisfies specified filters.
func (api *API) List(args params.ListEndpointsFiltersSets) (params.ListEndpointsItemsResults, error) {

	// This func constructs individual set of filters.
	filters := func(aSet params.ListEndpointsFiltersSet) []model.RemoteServiceFilter {
		result := make([]model.RemoteServiceFilter, len(aSet.Filters))
		for i, filter := range aSet.Filters {
			result[i] = constructRemoteServiceFilter(filter)
		}
		return result
	}

	// This func converts results for a filters set to params.
	convertToParams := func(remotes []model.RemoteService) []params.ListEndpointsServiceItemResult {
		results := make([]params.ListEndpointsServiceItemResult, len(remotes))
		for i, one := range remotes {
			results[i] = api.getRemoteService(one)
		}
		return results
	}

	found := make([]params.ListEndpointsItemsResult, len(args.Filters))
	for i, set := range args.Filters {
		setResult, err := api.backend.ListRemoteServices(filters(set)...)
		if err != nil {
			found[i].Error = common.ServerError(err)
			continue
		}
		found[i].Result = convertToParams(setResult)
	}

	return params.ListEndpointsItemsResults{found}, nil
}

func (api *API) getRemoteService(remote model.RemoteService) params.ListEndpointsServiceItemResult {
	service, err := api.access.Service(remote.ServiceName)
	if err != nil {
		return params.ListEndpointsServiceItemResult{Error: common.ServerError(err)}
	}

	ch, _, err := service.Charm()
	if err != nil {
		return params.ListEndpointsServiceItemResult{Error: common.ServerError(err)}
	}
	result := params.ListEndpointsServiceItem{
		Endpoints:   remote.Endpoints,
		CharmName:   ch.Meta().Name,
		ServiceName: remote.ServiceName,
		ServiceURL:  remote.ServiceURL,
		UsersCount:  len(remote.ConnectedUsers),
	}
	return params.ListEndpointsServiceItemResult{Result: &result}
}

func constructRemoteServiceFilter(filter params.ListEndpointsFilter) model.RemoteServiceFilter {
	return model.RemoteServiceFilter{
		ServiceURL: filter.ServiceURL,
		CharmName:  filter.CharmName,
		Endpoint: model.RemoteEndpointFilter{
			Name:      filter.Endpoint.Name,
			Interface: filter.Endpoint.Interface,
			Role:      filter.Endpoint.Role,
		},
	}
}

// parseOffer is a helper function that translates from params
// structure into internal service layer one.
func (api *API) parseOffer(p params.RemoteServiceOffer) (model.ServiceOffer, error) {
	service, err := api.access.Service(p.ServiceName)
	if err != nil {
		if errors.IsNotFound(err) {
			return model.ServiceOffer{}, common.ErrPerm
		}
		return model.ServiceOffer{}, errors.Annotatef(err, "getting service %v", p.ServiceName)
	}

	endpoints, err := getServiceEndpoints(service, p.Endpoints)
	if err != nil {
		return model.ServiceOffer{}, errors.Trace(err)
	}
	offer := model.ServiceOffer{
		ServiceURL:         p.ServiceURL,
		ServiceName:        service.Name(),
		Endpoints:          endpoints,
		ServiceDescription: p.ServiceDescription,
	}

	if p.ServiceDescription == "" {
		ch, _, err := service.Charm()
		if err != nil {
			return model.ServiceOffer{}, errors.Annotatef(err, "getting charm for service %v", p.ServiceName)
		}
		offer.ServiceDescription = ch.Meta().Description
	}

	return offer, nil
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

// convertServiceOffer is a helper function that translates from internal service layer
// structure into params one.
func convertServiceOffer(c model.ServiceOffer) params.ServiceOffer {
	return params.ServiceOffer{
		ServiceName:        c.ServiceName,
		ServiceURL:         c.ServiceURL,
		SourceEnvironTag:   names.NewEnvironTag(c.SourceEnvUUID).String(),
		SourceLabel:        c.SourceLabel,
		Endpoints:          convertEndpoints(c.Endpoints),
		ServiceDescription: c.ServiceDescription,
	}
}

func convertEndpoints(endpoints []charm.Relation) []params.RemoteEndpoint {
	remoteEndpoints := make([]params.RemoteEndpoint, len(endpoints))

	for i, endpoint := range endpoints {
		remoteEndpoints[i] = params.RemoteEndpoint{
			Name:      endpoint.Name,
			Interface: endpoint.Interface,
			Role:      endpoint.Role,
			Limit:     endpoint.Limit,
			Scope:     endpoint.Scope,
		}
	}
	return remoteEndpoints
}

// A ServicesBackend holds interface that this api requires.
// TODO (anastasiamac 2015-11-16) this may change as back-end actually materializes.
type ServicesBackend interface {

	// AddOffer adds a new service offer to the directory.
	AddOffer(offer model.ServiceOffer) error

	// ListOffers returns offers satisfying the specified filter.
	ListOffers(filter ...model.ServiceOfferFilter) ([]model.ServiceOffer, error)

	// ListRemoteServices returns remote services satisfying specified filters.
	ListRemoteServices(filters ...model.RemoteServiceFilter) ([]model.RemoteService, error)
}

// TODO (anastasiamac 2015-11-16) Remove me when backend is done
type servicesBackendStub struct{}

func (e *servicesBackendStub) AddOffer(offer model.ServiceOffer) error {
	return nil
}

func (e *servicesBackendStub) ListOffers(filter ...model.ServiceOfferFilter) ([]model.ServiceOffer, error) {
	return nil, nil
}

func (e *servicesBackendStub) ListRemoteServices(filters ...model.RemoteServiceFilter) ([]model.RemoteService, error) {
	return nil, nil
}
