// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"github.com/juju/juju/api/base"
)

// PatchResponses changes the internal FacadeCaller to one that lets you return
// canned results. The responseFunc will get the 'response' interface object,
// and can set attributes of it to fix the response to the caller.
// It can also return an error to have the FacadeCall return an error.
// The function returned by PatchResponses is a cleanup function that returns
// the client to its original state.
func PatchResponses(client *Client, responseFunc func(interface{}) error) func() {
	orig := client.facade
	client.facade = &resultCaller{responseFunc}
	return func() {
		client.facade = orig
	}
}

type resultCaller struct {
	setResult func(interface{}) error
}

func (f *resultCaller) FacadeCall(request string, params, response interface{}) error {
	return f.setResult(response)
}

func (f *resultCaller) BestAPIVersion() int {
	return 0
}

func (f *resultCaller) RawAPICaller() base.APICaller {
	return nil
}
