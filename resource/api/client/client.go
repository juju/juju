// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/loggo"
	"io"
)

var logger = loggo.GetLogger("juju.resource.api.client")

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

type rawAPI interface {
	facadeCaller
	io.Closer
}

// Client is the public client for the resources API facade.
type Client struct {
	*specClient
}

// NewClient returns a new Client for the given raw API caller.
func NewClient(raw rawAPI) *Client {
	return &Client{
		specClient: &specClient{raw},
	}
}
