// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
)

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	client *httprequest.Client
}

// MakeClient is a direct constructor function for a backups client.
func MakeClient(frontend base.ClientFacade, backend base.FacadeCaller, client *httprequest.Client) *Client {
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		client:       client,
	}
}

// NewClient returns a new backups API client.
func NewClient(st base.APICallCloser) (*Client, error) {
	frontend, backend := base.NewClientFacade(st, "Backups")
	client, err := st.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return MakeClient(frontend, backend, client), nil
}
