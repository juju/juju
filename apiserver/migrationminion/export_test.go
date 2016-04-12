// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import "github.com/juju/juju/state"

func PatchState(p Patcher, st Backend) {
	p.PatchValue(&getBackend, func(*state.State) Backend {
		return st
	})
}

type Patcher interface {
	PatchValue(ptr, value interface{})
}
