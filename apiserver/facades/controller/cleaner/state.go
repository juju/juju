// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"context"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state"
)

type StateInterface interface {
	Cleanup(context.Context, objectstore.ObjectStore, state.MachineRemover) error
	WatchCleanups() state.NotifyWatcher
}

type stateShim struct {
	*state.State
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
