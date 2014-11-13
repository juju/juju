// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"net/http"

	"github.com/juju/juju/api/base"
)

type httpClient interface {
	SendHTTPRequest(method, path string, args interface{}) (*http.Request, *http.Response, error)
}

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	http   httpClient
}

type httpAPICallCloser interface {
	base.APICallCloser
	httpClient
}

// NewClient returns a new backups API client.
func NewClient(st httpAPICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Backups")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		http:         st,
	}
}
