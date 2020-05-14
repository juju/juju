// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import "github.com/juju/juju/api/base"

const charmHubFacade = "CharmHub"

// Client allows access to the charmhub API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charmhub api.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, charmHubFacade)
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}
