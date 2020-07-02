// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const charmHubFacade = "CharmHub"

// Client allows access to the charmhub API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charmhub api.
func NewClient(callCloser base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(callCloser, charmHubFacade)
	return newClientFromFacade(frontend, backend)
}

// NewClientFromFacade creates a new charmhub client using the input
// client facade and facade caller.
func newClientFromFacade(frontend base.ClientFacade, backend base.FacadeCaller) *Client {
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}

// Info queries the charmhub API for information for a given name.
func (c *Client) Info(name string) (InfoResponse, error) {
	args := params.Entity{Tag: names.NewApplicationTag(name).String()}
	var result params.CharmHubCharmInfoResult
	if err := c.facade.FacadeCall("Info", args, &result); err != nil {
		return InfoResponse{}, errors.Trace(err)
	}

	return convertCharmInfoResult(result.Result), nil
}

// Find queries the charmhub API finding potential charms or bundles for the
// given query.
func (c *Client) Find(query string) ([]FindResponse, error) {
	args := params.Query{Query: query}
	var result params.CharmHubCharmFindResult
	if err := c.facade.FacadeCall("Find", args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return convertCharmFindResults(result.Results), nil
}
