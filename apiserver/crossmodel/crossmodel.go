// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

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
	exporter   crossmodel.Exporter
}

// createAPI returns a new cross model API facade.
func createAPI(
	exporter crossmodel.Exporter,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		authorizer: authorizer,
		exporter:   exporter,
	}, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(exporter(st), resources, authorizer)
}

func exporter(st *state.State) crossmodel.Exporter {
	return crossmodel.ExporterStub{}
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(all params.CrossModelOffers) (params.ErrorResults, error) {
	// export service offers
	offers := make([]params.ErrorResult, len(all.Offers))
	for i, one := range all.Offers {
		offer, err := parseOffer(one)
		if err != nil {
			offers[i].Error = common.ServerError(err)
			continue
		}

		if err := api.exporter.ExportOffer(offer); err != nil {
			offers[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: offers}, nil
}

// Show gets details about remote services that match given URLs.
func (api *API) Show(urls []string) (params.RemoteServiceInfoResults, error) {
	found, err := api.exporter.Search(urls)
	if err != nil {
		return params.RemoteServiceInfoResults{}, errors.Trace(err)
	}
	if len(found) == 0 {
		return params.RemoteServiceInfoResults{}, errors.NotFoundf("endpoints with urls %v", strings.Join(urls, ","))
	}

	results := make([]params.RemoteServiceInfoResult, len(found))
	for i, one := range found {
		results[i].RemoteService = convertRemoteServiceDetails(one)
		// TODO (anastasiamac 2015-11-12) once back-end is done in separate branch,
		// fix values for results[i].URL and results[i].Error
	}
	return params.RemoteServiceInfoResults{results}, nil
}

// parseOffer is a helper function that translates from params
// structure into internal service layer one.
func parseOffer(p params.CrossModelOffer) (crossmodel.Offer, error) {
	offer := crossmodel.Offer{}

	serviceTag, err := names.ParseServiceTag(p.Service)
	if err != nil {
		return offer, errors.Annotatef(err, "cannot parse service tag %q", p.Service)
	}

	users := make([]names.UserTag, len(p.Users))
	for i, user := range p.Users {
		users[i], err = names.ParseUserTag(user)
		if err != nil {
			return offer, errors.Annotatef(err, "cannot parse user tag %q", user)
		}
	}
	offer.Service = serviceTag
	offer.URL = p.URL
	offer.Users = users
	offer.Endpoints = p.Endpoints
	return offer, nil
}

// convertRemoteServiceDetails is a helper function that translates from internal service layer
// structure into params one.
func convertRemoteServiceDetails(c crossmodel.RemoteServiceEndpoints) params.RemoteServiceInfo {
	endpoints := make([]params.RemoteEndpoint, len(c.Endpoints))

	for i, endpoint := range c.Endpoints {
		endpoints[i] = params.RemoteEndpoint{
			Name:      endpoint.Name,
			Interface: endpoint.Interface,
			Role:      endpoint.Role,
		}
	}
	return params.RemoteServiceInfo{
		Service:     c.Service.String(),
		Endpoints:   endpoints,
		Description: c.Description,
	}
}
