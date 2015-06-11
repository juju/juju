// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import "github.com/juju/juju/state"

type StateInterface interface {
	Cleanup() error
	WatchCleanups() state.NotifyWatcher
}

type stateShim struct {
	*state.State
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
