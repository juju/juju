// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
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

	err := c.facade.FacadeCall("Show", []string{url}, &found)
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

func (c *Client) List(filters map[string][]string) (map[string][]params.ListEndpointsServiceItemResult, error) {
	// TODO (anastasiamac 2015-11-18) construct meaningful filters from input
	in := params.ListEndpointsFilters{}
	out := params.ListEndpointsServiceItemResults{}

	err := c.facade.FacadeCall("List", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return out.Results, nil
}
