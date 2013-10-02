// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// State provides access to an upgrader worker's view of the state.
type State struct {
	caller common.Caller
}

// NewState returns a version of the state that provides functionality
// required by the upgrader worker.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

// SetTools sets the tools associated with the entity
// with the given tag, which must be the tag
// of the entity that the upgrader is running
// on behalf of.
func (st *State) SetTools(tag string, tools *tools.Tools) error {
	var results params.ErrorResults
	args := params.SetAgentsTools{
		AgentTools: []params.SetAgentTools{{
			Tag:   tag,
			Tools: tools,
		}},
	}
	err := st.caller.Call("Upgrader", "", "SetTools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return err
	}
	return results.OneError()
}

func (st *State) DesiredVersion(tag string) (version.Number, error) {
	var results params.AgentVersionResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Upgrader", "", "DesiredVersion", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return version.Number{}, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return version.Number{}, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return version.Number{}, err
	}
	if result.Version == nil {
		// TODO: Not directly tested
		return version.Number{}, fmt.Errorf("received no error, but got a nil Version")
	}
	return *result.Version, nil
}

// Tools returns the agent tools for the given entity.
func (st *State) Tools(tag string) (*tools.Tools, error) {
	var results params.AgentToolsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Upgrader", "", "Tools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (st *State) WatchAPIVersion(agentTag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag}},
	}
	err := st.caller.Call("Upgrader", "", "WatchAPIVersion", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.caller, result)
	return w, nil
}
