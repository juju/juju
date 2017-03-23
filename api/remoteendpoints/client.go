// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoteendpoints

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
)

// Client allows access to the cross model management API end points.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the cross model relations API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "RemoteEndpoints")
	return &Client{ClientFacade: frontend, facade: backend}
}

// ApplicationOffer returns offered remote application details for a given URL.
func (c *Client) ApplicationOffer(urlStr string) (params.ApplicationOffer, error) {

	url, err := crossmodel.ParseApplicationURL(urlStr)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}
	if url.Source != "" {
		return params.ApplicationOffer{}, errors.NotSupportedf("query for non-local application offers")
	}

	found := params.ApplicationOffersResults{}

	err = c.facade.FacadeCall("ApplicationOffers", params.ApplicationURLs{[]string{urlStr}}, &found)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}

	result := found.Results
	if len(result) != 1 {
		return params.ApplicationOffer{}, errors.Errorf("expected to find one result for url %q but found %d", url, len(result))
	}

	theOne := result[0]
	if theOne.Error != nil {
		return params.ApplicationOffer{}, errors.Trace(theOne.Error)
	}
	return theOne.Result, nil
}

// FindApplicationOffers returns all application offers matching the supplied filter.
func (c *Client) FindApplicationOffers(filters ...crossmodel.ApplicationOfferFilter) ([]params.ApplicationOffer, error) {
	// We need at least one filter. The default filter will list all local applications.
	if len(filters) == 0 {
		return nil, errors.New("at least one filter must be specified")
	}
	var paramsFilter params.OfferFilters
	for _, f := range filters {
		filterTerm := params.OfferFilter{
			OfferName: f.OfferName,
			ModelName: f.ModelName,
			OwnerName: f.OwnerName,
		}
		filterTerm.Endpoints = make([]params.EndpointFilterAttributes, len(f.Endpoints))
		for i, ep := range f.Endpoints {
			filterTerm.Endpoints[i].Name = ep.Name
			filterTerm.Endpoints[i].Interface = ep.Interface
			filterTerm.Endpoints[i].Role = ep.Role
		}
		paramsFilter.Filters = append(paramsFilter.Filters, filterTerm)
	}

	out := params.FindApplicationOffersResults{}
	err := c.facade.FacadeCall("FindApplicationOffers", paramsFilter, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}
