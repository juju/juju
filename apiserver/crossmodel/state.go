// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/state"
)

type stateAccess interface {
}

var GetServiceDirectoryState = func(st *state.State) stateAccess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}
