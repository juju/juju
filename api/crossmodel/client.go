// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
)

var logger = loggo.GetLogger("juju.api.crossmodel")

// Client allows access to the cross model management API end points.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the cross model relations API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "CrossModelRelations")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Offer prepares service's endpoints for consumption.
func (c *Client) Offer(service string, endpoints []string, url string, users []string, desc string) ([]params.ErrorResult, error) {
	offers := []params.RemoteServiceOffer{
		params.RemoteServiceOffer{
			ServiceName:        service,
			ServiceDescription: desc,
			Endpoints:          endpoints,
			ServiceURL:         url,
			AllowedUserTags:    users,
		},
	}
	out := params.ErrorResults{}
	if err := c.facade.FacadeCall("Offer", params.RemoteServiceOffers{Offers: offers}, &out); err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

// Show returns offered remote service details for a given URL.
func (c *Client) Show(url string) (params.ServiceOffer, error) {
	found := params.RemoteServiceResults{}

	err := c.facade.FacadeCall("Show", params.ShowFilter{[]string{url}}, &found)
	if err != nil {
		return params.ServiceOffer{}, errors.Trace(err)
	}

	result := found.Results
	if len(result) != 1 {
		return params.ServiceOffer{}, errors.Errorf("expected to find one result for url %q but found %d", url, len(result))
	}

	theOne := result[0]
	if theOne.Error != nil {
		return params.ServiceOffer{}, errors.Trace(theOne.Error)
	}
	return theOne.Result, nil
}

// List gets all remote services that have been offered from this Juju model.
// Each returned service satisfies at least one of the the specified filters.
func (c *Client) List(filters ...crossmodel.RemoteServiceFilter) ([]crossmodel.ListEndpointsServiceResult, error) {
	// TODO (anastasiamac 2015-11-23) translate a set of filters from crossmodel domain to params
	paramsFilters := params.ListEndpointsFilters{
		Filters: []params.ListEndpointsFilter{
			{FilterTerms: []params.ListEndpointsFilterTerm{}},
		},
	}

	out := params.ListEndpointsItemsResults{}

	err := c.facade.FacadeCall("List", paramsFilters, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := out.Results
	// Since only one filters set was sent, expecting only one back
	if len(result) != 1 {
		return nil, errors.Errorf("expected to find one result but found %d", len(result))

	}

	theOne := result[0]
	if theOne.Error != nil {
		return nil, errors.Trace(theOne.Error)
	}

	return convertListResultsToModel(theOne.Result), nil
}

func convertListResultsToModel(items []params.ListEndpointsServiceItemResult) []crossmodel.ListEndpointsServiceResult {
	result := make([]crossmodel.ListEndpointsServiceResult, len(items))
	for i, one := range items {
		if one.Error != nil {
			result[i].Error = one.Error
			continue
		}
		remoteService := one.Result
		result[i].Result = &crossmodel.ListEndpointsService{
			ServiceName:    remoteService.ServiceName,
			ServiceURL:     remoteService.ServiceURL,
			CharmName:      remoteService.CharmName,
			ConnectedCount: remoteService.UsersCount,
			Endpoints:      remoteService.Endpoints,
		}
	}
	return result
}
