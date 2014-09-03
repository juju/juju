// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/juju/api/base"
)

// PatchFacadeCall patches the State's facade such that
// FacadeCall method calls are diverted to the provided
// function.
func PatchFacadeCall(p patcher, st *State, f func(request string, params, response interface{}) error) {
	p.PatchValue(&st.facade, &facadeWrapper{st.facade, f})
}

type patcher interface {
	PatchValue(dest, value interface{})
}

type facadeWrapper struct {
	base.FacadeCaller
	facadeCall func(request string, params, response interface{}) error
}

func (f *facadeWrapper) FacadeCall(request string, params, response interface{}) error {
	return f.facadeCall(request, params, response)
}
