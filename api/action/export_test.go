// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/juju/api/base"
)

// ExposeFacade returns the client's underlying FacadeCaller.
func ExposeFacade(c *Client) base.FacadeCaller {
	return c.facade
}

// PatchClientFacadeCall changes the internal FacadeCaller to one that lets
// the user mock out the FacadeCall method. The function returned by
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
	return nil
}
