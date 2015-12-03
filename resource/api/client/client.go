// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"io"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.resource.api.client")

// TODO(ericsnow) Move FacadeCaller to a component-central package.

// FacadeCaller has the api/base.FacadeCaller methods needed for the component.
type FacadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// Client is the public client for the resources API facade.
type Client struct {
	// Add the sub-client here.
	io.Closer
}

// NewClient returns a new Client for the given raw API caller.
func NewClient(caller FacadeCaller, closer io.Closer) *Client {
	return &Client{
		// Add the sub-client here.
		Closer:     closer,
	}
}

func (c Client) Upload(service, name string, resource io.Reader) error {
	return nil
}
