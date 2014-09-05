// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/api/base"
)

// PatchFacadeCall patches the provided FacadeCaller such
// that the FacadeCall method calls are diverted to the
// provided function.
func PatchFacadeCall(p Patcher, caller *base.FacadeCaller, f func(request string, params, response interface{}) error) {
	p.PatchValue(caller, &facadeWrapper{*caller, f})
}

type Patcher interface {
	PatchValue(dest, value interface{})
}

type facadeWrapper struct {
	base.FacadeCaller
	facadeCall func(request string, params, response interface{}) error
}

func (f *facadeWrapper) FacadeCall(request string, params, response interface{}) error {
	return f.facadeCall(request, params, response)
}
