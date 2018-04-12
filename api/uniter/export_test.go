// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
)

var (
	NewSettings = newSettings
)

// PatchUnitResponse changes the internal FacadeCaller to one that lets you return
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

// CreateUnit creates uniter.Unit for tests.
func CreateUnit(st *State, tag names.UnitTag) *Unit {
	return &Unit{
		st:           st,
		tag:          tag,
		life:         params.Alive,
		resolvedMode: params.ResolvedNone,
		series:       "trusty",
	}
}

var NewStateV4 = newStateForVersionFn(4)
