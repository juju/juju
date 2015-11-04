// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
)

var logger = loggo.GetLogger("juju.api.backups")

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	client *httprequest.Client
}

// NewClient returns a new backups API client.
func NewClient(st base.APICallCloser) (*Client, error) {
	frontend, backend := base.NewClientFacade(st, "Backups")
	client, err := st.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		client:       client,
	}, nil
}
