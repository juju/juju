// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// State provides access to the Machiner API facade.
type State struct {
	caller common.Caller
}

// NewState creates a new client-side Machiner facade.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

// entityLife requests the lifecycle of the given machine from the server.
func (st *State) entityLife(tag string) (params.Life, error) {
	var result params.LifeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Machiner", "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Results) != 1 {
		return "", fmt.Errorf("expected one result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return "", err
	}
	return result.Results[0].Life, nil
}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(tag string) (*Machine, error) {
	life, err := st.entityLife(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Environment returns the Environment corresponding
// to the machine with the given tag.
func (st *State) Environment() (*Environment, error) {
	var result params.MachineEnvironmentResult
	err := st.caller.Call("Machiner", "", "Environment", nil, &result)
	if err != nil {
		return nil, err
	}
	return &Environment{
		tag:  result.EnvironmentTag,
		life: result.Life,
		st:   st,
	}, nil
}

// watch returns a watcher for observing changes to the given entity.
func (st *State) watch(tag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Machiner", "", "Watch", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return watcher.NewNotifyWatcher(st.caller, result), nil
}
