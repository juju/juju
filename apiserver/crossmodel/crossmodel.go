// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("CrossModelRelations", 1, NewAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer common.Authorizer
	directory  crossmodel.ServiceDirectory
	access     stateAccess
}

// createAPI returns a new cross model API facade.
func createAPI(
	directory crossmodel.ServiceDirectory,
	access stateAccess,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		authorizer: authorizer,
		directory:  directory,
		access:     access,
	}, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(serviceDirectory(st), getStateAccess(st), resources, authorizer)
}

func serviceDirectory(st *state.State) crossmodel.ServiceDirectory {
	return state.NewServiceDirectory(st)
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(all params.CrossModelOffers) (params.ErrorResults, error) {
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

		if err := api.directory.AddOffer(offer); err != nil {
			offers[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: offers}, nil
}

// Show gets details about remote services that match given URLs.
func (api *API) Show(urls []string) (params.RemoteServiceInfoResults, error) {
	filters := make([]crossmodel.ServiceOfferFilter, len(urls))
	for i, one := range urls {
		filters[i].ServiceURL = one
	}

	found, err := api.directory.ListOffers(filters...)
	if err != nil {
		return params.RemoteServiceInfoResults{}, errors.Trace(err)
	}

	tpMap := make(map[string]crossmodel.ServiceOffer, len(found))
	for _, offer := range found {
		tpMap[offer.ServiceURL] = offer
	}

	results := make([]params.RemoteServiceInfoResult, len(urls))
	for i, one := range urls {
		foundOffer, ok := tpMap[one]
		if !ok {
			results[i].Error = common.ServerError(errors.NotFoundf("offer for remote service url %v", one))
			continue
		}
		results[i].Result = convertServiceOffer(foundOffer)
	}
	return params.RemoteServiceInfoResults{results}, nil
}

// parseOffer is a helper function that translates from params
// structure into internal service layer one.
func (api *API) parseOffer(p params.CrossModelOffer) (crossmodel.ServiceOffer, error) {
	service, err := api.access.Service(p.Service)
	if err != nil {
		if errors.IsNotFound(err) {
			return crossmodel.ServiceOffer{}, common.ErrPerm
		}
		return crossmodel.ServiceOffer{}, errors.Annotatef(err, "getting service %v", p.Service)
	}

	endpoints, err := getEndpointsOnOffer(service, set.NewStrings(p.Endpoints...))
	if err != nil {
		return crossmodel.ServiceOffer{}, errors.Trace(err)
	}

	ch, _, err := service.Charm()
	if err != nil {
		return crossmodel.ServiceOffer{}, errors.Annotatef(err, "getting charm for service %v", p.Service)
	}

	return crossmodel.ServiceOffer{
		ServiceURL:         p.URL,
		ServiceName:        service.Name(),
		Endpoints:          endpoints,
		ServiceDescription: ch.Meta().Description,
	}, nil
}

func getEndpointsOnOffer(service *state.Service, points set.Strings) ([]charm.Relation, error) {
	rs, err := service.Relations()
	if err != nil {
		return nil, errors.Annotatef(err, "getting relations for service %v", service.Name())
	}
	result := []charm.Relation{}
	for _, r := range rs {
		endpoint, err := r.Endpoint(service.Name())
		if err != nil {
			// TODO (anastasiamac 2015-11-13) I am not convinced that we care about this error here
			// as it might be related to an endpoint that we are not exporting anyway...
			return nil, errors.Annotatef(err, "getting relation endpoint for relation %v and service %v", r, service.Name())
		}
		if points.Contains(endpoint.Name) {
			result = append(result, endpoint.Relation)
		}
	}
	return result, nil
}

// convertServiceOffer is a helper function that translates from internal service layer
// structure into params one.
func convertServiceOffer(c crossmodel.ServiceOffer) params.RemoteServiceInfo {
	endpoints := make([]params.RemoteEndpoint, len(c.Endpoints))

	for i, endpoint := range c.Endpoints {
		endpoints[i] = params.RemoteEndpoint{
			Name:      endpoint.Name,
			Interface: endpoint.Interface,
			Role:      string(endpoint.Role),
		}
	}
	return params.RemoteServiceInfo{
		Service:     c.ServiceName,
		Endpoints:   endpoints,
		Description: c.ServiceDescription,
	}
}
