// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/juju/api/base/testing"
)

// PatchFacadeCall patches the State's facade such that
// FacadeCall method calls are diverted to the provided
// function.
func PatchFacadeCall(p testing.Patcher, st State, f func(request string, params, response interface{}) error) {
	st0 := st.(*state) // *state is the only implementation of State.
	testing.PatchFacadeCall(p, &st0.facade, f)
}
