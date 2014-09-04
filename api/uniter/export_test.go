// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package uniter

import (
	"github.com/juju/juju/api/base/testing"
)

var NewSettings = newSettings

// PatchResponses changes the internal FacadeCaller to one that lets you return
// canned results. The responseFunc will get the 'response' interface object,
// and can set attributes of it to fix the response to the caller.
// It can also return an error to have the FacadeCall return an error.
// The function returned by PatchResponses is a cleanup function that returns
// the client to its original state.
func PatchUnitResponse(p testing.Patcher, u *Unit, responseFunc func(interface{}) error) {
	testing.PatchFacadeCall(p, &u.st.facade, func(request string, params, response interface{}) error {
		return responseFunc(response)
	})
}
