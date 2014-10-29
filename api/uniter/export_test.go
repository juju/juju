// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package uniter

import (
	"fmt"

	"github.com/juju/juju/api/base/testing"
)

var (
	NewSettings = newSettings
	NewStateV0  = newStateV0
	NewStateV1  = newStateV1
)

// PatchResponses changes the internal FacadeCaller to one that lets you return
// canned results. The responseFunc will get the 'response' interface object,
// and can set attributes of it to fix the response to the caller.
// It can also return an error to have the FacadeCall return an error. The expected
// request is specified using the expectedRequest parameter. If the request name does
// not match, the function panics.
// The function returned by PatchResponses is a cleanup function that returns
// the client to its original state.
func PatchUnitResponse(p testing.Patcher, u *Unit, expectedRequest string, responseFunc func(interface{}) error) {
	testing.PatchFacadeCall(p, &u.st.facade, func(request string, params, response interface{}) error {
		if request != expectedRequest {
			panic(fmt.Errorf("unexpected request %q received - expecting %q", request, expectedRequest))
		}
		return responseFunc(response)
	})
}
