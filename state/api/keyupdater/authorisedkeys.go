// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"fmt"

	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// State provides access to a worker's view of the state.
type State struct {
	caller base.Caller
}

func (st *State) call(method string, params, result interface{}) error {
	return st.caller.Call("KeyUpdater", "", method, params, result)
}

// NewState returns a version of the state that provides functionality required by the worker.
func NewState(caller base.Caller) *State {
	return &State{caller}
}

// AuthorisedKeys returns the authorised ssh keys for the machine specified by machineTag.
func (st *State) AuthorisedKeys(machineTag string) ([]string, error) {
	var results params.StringsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag}},
	}
	err := st.call("AuthorisedKeys", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// WatchAuthorisedKeys returns a notify watcher that looks for changes in the
// authorised ssh keys for the machine specified by machineTag.
func (st *State) WatchAuthorisedKeys(machineTag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag}},
	}
	err := st.call("WatchAuthorisedKeys", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.caller, result)
	return w, nil
}
