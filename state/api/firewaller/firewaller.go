// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// State provides access to the Firewaller API facade.
type State struct {
	caller base.Caller
}

// NewState creates a new client-side Firewaller facade.
func NewState(caller base.Caller) *State {
	return &State{caller}
}

// life requests the life cycle of the given entity from the server.
func (st *State) life(tag string) (params.Life, error) {
	return common.Life(st.caller, "Firewaller", tag)
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

// WatchForEnvironConfigChanges return a NotifyWatcher waiting for the
// environment configuration to change.
func (st *State) WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := st.caller.Call("Firewaller", "", "WatchForEnvironConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.caller, result)
	return w, nil
}

// EnvironConfig returns the current environment configuration.
func (st *State) EnvironConfig() (*config.Config, error) {
	var result params.EnvironConfigResult
	err := st.caller.Call("Firewaller", "", "EnvironConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// environment.
func (st *State) WatchEnvironMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.caller.Call("Firewaller", "", "WatchEnvironMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.caller, result)
	return w, nil
}
