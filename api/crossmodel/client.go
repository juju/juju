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

// NewClient creates a new client for accessing the cross model API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "CrossModel")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Offer prepares service's endpoints for consumption.
func (c *Client) Offer(service string, endpoints []string, url string, users []string) error {
	in := params.CrossModelOffer{service, endpoints, url, users}
	return errors.Trace(c.facade.FacadeCall("Offer", in, nil))
}
