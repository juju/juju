// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// State provides access to an upgrader worker's view of the state.
type State struct {
	caller common.Caller
}

// NewState returns a version of the state that provides functionality
// required by the upgrader worker.
func New(caller common.Caller) *State {
	return &State{caller}
}

func (st *State) SetTools(tools params.AgentTools) error {
	var results params.SetAgentToolsResults
	args := params.SetAgentTools{
		AgentTools: []params.AgentTools{tools},
	}
	err := st.caller.Call("Upgrader", "", "SetTools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return err
	}
	if len(results.Results) != 1 {
		return fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Tag != tools.Tag {
		// TODO: Not directly tested
		return fmt.Errorf("server returned tag that did not match: got %q expected %q",
			result.Tag, tools.Tag)
	}
	if err := result.Error; err != nil {
		return err
	}
	return nil
}

func (st *State) Tools(tag string) (*params.AgentTools, error) {
	var results params.AgentToolsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Upgrader", "", "Tools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Tools) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected one result, got %d", len(results.Tools))
	}
	tools := results.Tools[0]
	if err := tools.Error; err != nil {
		return nil, err
	}
	if tools.AgentTools.Tag != tag {
		// TODO: Not directly tested
		return nil, fmt.Errorf("server returned tag that did not match: got %q expected %q",
			tools.AgentTools.Tag, tag)
	}
	return &tools.AgentTools, nil
}

func (st *State) WatchAPIVersion(agentTag string) (*watcher.NotifyWatcher, error) {
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
