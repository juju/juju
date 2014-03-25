// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

const firewallerFacade = "Firewaller"

// State provides access to the Firewaller API facade.
type State struct {
	caller base.Caller
	*common.EnvironWatcher
}

func (st *State) call(method string, params, result interface{}) error {
	return st.caller.Call(firewallerFacade, "", method, params, result)
}

// NewState creates a new client-side Firewaller facade.
func NewState(caller base.Caller) *State {
	return &State{
		caller:         caller,
		EnvironWatcher: common.NewEnvironWatcher(firewallerFacade, caller),
	}
}

// life requests the life cycle of the given entity from the server.
func (st *State) life(tag string) (params.Life, error) {
	return common.Life(st.caller, firewallerFacade, tag)
}

// Unit provides access to methods of a state.Unit through the facade.
func (st *State) Unit(tag string) (*Unit, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (st *State) Machine(tag string) (*Machine, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// environment.
func (st *State) WatchEnvironMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.call("WatchEnvironMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.caller, result)
	return w, nil
}
