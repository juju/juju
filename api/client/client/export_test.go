// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
)

// PatchClientFacadeCall changes the internal FacadeCaller to one that lets
// you mock out the FacadeCall method. The function returned by
// PatchClientFacadeCall is a cleanup function that returns the client to its
// original state.
func PatchClientFacadeCall(c *Client, mockCall func(request string, params interface{}, response interface{}) error) func() {
	orig := c.facade
	c.facade = &resultCaller{mockCall}
	return func() {
		c.facade = orig
	}
}

type resultCaller struct {
	mockCall func(request string, params interface{}, response interface{}) error
}

func (f *resultCaller) FacadeCall(request string, params, response interface{}) error {
	return f.mockCall(request, params, response)
}

func (f *resultCaller) Name() string {
	return ""
}

func (f *resultCaller) BestAPIVersion() int {
	return 0
}

func (f *resultCaller) RawAPICaller() base.APICaller {
	return &rawAPICaller{}
}

type rawAPICaller struct {
	base.APICaller
}

func (r *rawAPICaller) Context() context.Context {
	return context.Background()
}

// SetServerAddress allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerAddress(c *Client, scheme, addr string) {
	api.SetServerAddressForTesting(c.conn, scheme, addr)
}

// APIClient returns a 'barebones' api.Client suitable for calling FindTools in
// an error state (anything else is likely to panic.)
func BarebonesClient(apiCaller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(apiCaller, "Client")
	return &Client{ClientFacade: frontend, facade: backend, conn: api.EmptyConnectionForTesting()}
}
