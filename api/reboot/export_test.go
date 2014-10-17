package reboot

import (
	"github.com/juju/juju/api/base/testing"
)

// PatchFacadeCall patches the State's facade such that
// FacadeCall method calls are diverted to the provided
// function.
func PatchFacadeCall(p testing.Patcher, st *State, f func(request string, params, response interface{}) error) {
	testing.PatchFacadeCall(p, &st.facade, f)
}
