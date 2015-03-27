// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/juju/api/base"
)

// Client provides access to the spaces facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new spaces client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Space")
	return &Client{ClientFacade: frontend, facade: backend}
}
