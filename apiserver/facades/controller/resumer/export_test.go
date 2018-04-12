// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"github.com/juju/juju/state"
)

type StateInterface stateInterface

type Patcher interface {
	PatchValue(ptr, value interface{})
}

func PatchState(p Patcher, st StateInterface) {
	p.PatchValue(&getState, func(*state.State) stateInterface {
		return st
	})
}
