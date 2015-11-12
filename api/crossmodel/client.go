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

// Show returns offered remote service details for a given URL.
func (c *Client) Show(url string) (params.RemoteServiceInfo, error) {
	found := params.RemoteServiceInfoResults{}

	err := c.facade.FacadeCall("Show", []string{url}, &found)
	if err != nil {
		return params.RemoteServiceInfo{}, errors.Trace(err)
	}

	result := found.Results
	if len(result) > 1 {
		return params.RemoteServiceInfo{}, errors.Errorf("expected to find one result for url %q but found %d", url, len(result))
	}

	if len(result) == 0 {
		return params.RemoteServiceInfo{}, errors.NotFoundf("remote service with url %q", url)
	}

	theOne := result[0]
	if theOne.Error != nil {
		return params.RemoteServiceInfo{}, errors.Trace(theOne.Error)
	}
	return theOne.RemoteService, nil
}
