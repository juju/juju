// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/state"
)

type stateAccessor interface {
	WatchOfferedServices() state.StringsWatcher
}

type storageStateShim struct {
	*state.State
}

var getState = func(st *state.State) stateAccessor {
	return storageStateShim{st}
}
