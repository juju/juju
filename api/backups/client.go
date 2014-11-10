// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

type httpClient interface {
	SendHTTPRequest(method, path string, args interface{}) (*http.Request, *http.Response, error)
}

type httpAPICallCloser interface {
	base.APICallCloser
	httpClient
}

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	http   httpClient
	st     base.APICallCloser
}

// NewClient returns a new backups API client.
func NewClient(st httpAPICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Backups")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		http:         st,
		st:           st,
	}
}

func (c *Client) publicAddress() (string, error) {
	_, facade := base.NewClientFacade(c.st, "Client")

	// TODO (ericsnow) #1389362 Find the machine in a non-hardcoded way.
	args := params.PrivateAddress{Target: "0"}

	var results params.PrivateAddressResults
	err := facade.FacadeCall("PublicAddress", args, &results)
	return results.PublicAddress, errors.Trace(err)
}
