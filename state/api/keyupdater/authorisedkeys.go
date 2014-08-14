// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"fmt"

	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// State provides access to a worker's view of the state.
type State struct {
	facade base.FacadeCaller
}

// NewState returns a version of the state that provides functionality required by the worker.
func NewState(caller base.APICaller) *State {
	return &State{base.NewFacadeCaller(caller, "KeyUpdater")}
}

// AuthorisedKeys returns the authorised ssh keys for the machine specified by machineTag.
func (st *State) AuthorisedKeys(machineTag string) ([]string, error) {
	var results params.StringsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag}},
	}
	err := st.facade.FacadeCall("AuthorisedKeys", args, &results)
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
	err := st.facade.FacadeCall("WatchAuthorisedKeys", args, &results)
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
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
