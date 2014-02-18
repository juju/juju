// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// State provides access to an upgrader worker's view of the state.
type State struct {
	caller base.Caller
}

// NewState returns a version of the state that provides functionality
// required by the upgrader worker.
func NewState(caller base.Caller) *State {
	return &State{caller}
}

// SetVersion sets the tools version associated with the entity with
// the given tag, which must be the tag of the entity that the
// upgrader is running on behalf of.
func (st *State) SetVersion(tag string, v version.Binary) error {
	var results params.ErrorResults
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag:   tag,
			Tools: &params.Version{v},
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
	var results params.VersionResults
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

// Tools returns the agent tools that should run on the given entity,
// along with a flag whether to disable SSL hostname verification.
func (st *State) Tools(tag string) (*tools.Tools, bool, error) {
	var results params.ToolsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Upgrader", "", "Tools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, false, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, false, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, false, err
	}
	return result.Tools, result.DisableSSLHostnameVerification, nil
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
