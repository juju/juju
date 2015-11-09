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
func (c *Client) Offer(service string, endpoints []string, url string, users []string) ([]params.ErrorResult, error) {
	offers := []params.CrossModelOffer{
		params.CrossModelOffer{service, endpoints, url, users},
	}

	out := params.ErrorResults{}
	if err := c.facade.FacadeCall("Offer", params.CrossModelOffers{Offers: offers}, &out); err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

// Show returns offered endpoints details for a given URL.
func (c *Client) Show(url string) (params.EndpointsDetailsResult, error) {
	found := params.EndpointsDetailsResult{}
	filter := params.EndpointsSearchFilter{URL: url}
	if err := c.facade.FacadeCall("Show", filter, &found); err != nil {
		return params.EndpointsDetailsResult{}, errors.Trace(err)
	}
	return found, nil
}
