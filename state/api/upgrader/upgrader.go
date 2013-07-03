// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
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
		return err
	}
	if len(results.Results) != 1 {
		return fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	return nil
}
