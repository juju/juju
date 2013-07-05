// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Upgrader provides access to the Upgrader API facade.
type Upgrader struct {
	caller common.Caller
}

// New creates a new client-side Upgrader facade.
func New(caller common.Caller) *Upgrader {
	return &Upgrader{caller}
}

func (u *Upgrader) SetTools(tools params.AgentTools) error {
	var results params.SetAgentToolsResults
	args := params.SetAgentTools{
		AgentTools: []params.AgentTools{tools},
	}
	err := u.caller.Call("Upgrader", "", "SetTools", args, &results)
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

func (u *Upgrader) Tools(agentTag string) (*params.AgentTools, error) {
	var results params.AgentToolsResults
	args := params.Agents{
		Agents: []params.Agent{params.Agent{Tag: agentTag}},
	}
	err := u.caller.Call("Upgrader", "", "Tools", args, &results)
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
	if tools.AgentTools.Tag != agentTag {
		// TODO: Not directly tested
		return nil, fmt.Errorf("server returned tag that did not match: got %q expected %q",
			tools.AgentTools.Tag, agentTag)
	}
	return &tools.AgentTools, nil
}

func (u *Upgrader) WatchAPIVersion(agentTag string) (params.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Agents{
		Agents: []params.Agent{params.Agent{Tag: agentTag}},
	}
	err := u.caller.Call("Upgrader", "", "WatchAPIVersion", args, &results)
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
	w := watcher.NewNotifyWatcher(u.caller, result)
	return w, nil
}
