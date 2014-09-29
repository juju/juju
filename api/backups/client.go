// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/juju/api/base"
)

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new backups API client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Backups")
	return &Client{ClientFacade: frontend, facade: backend}
}
