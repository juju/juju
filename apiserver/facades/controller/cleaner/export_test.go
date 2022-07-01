// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/juju/v3/state"
)

var (
	NewCleanerAPI = newCleanerAPI
)

type Patcher interface {
	PatchValue(ptr, value interface{})
}

func PatchState(p Patcher, st StateInterface) {
	p.PatchValue(&getState, func(*state.State) StateInterface {
		return st
	})
}
