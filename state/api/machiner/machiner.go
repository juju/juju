// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// State provides access to a machiner worker's view of the state.
type State struct {
	caller common.Caller
}

// Machiner returns a version of the state that provides functionality
// required by the machiner worker.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

// machineLife requests the lifecycle of the given machine from the server.
func (m *State) machineLife(tag string) (params.Life, error) {
	var result params.MachinesLifeResults
	args := params.Entities{
		Entities: []params.Entity{{tag}},
	}
	err := m.caller.Call("Machiner", "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Machines) != 1 {
		return "", fmt.Errorf("expected one result, got %d", len(result.Machines))
	}
	if err := result.Machines[0].Error; err != nil {
		return "", err
	}
	return result.Machines[0].Life, nil
}

// Machine provides access to methods of a state.Machine through the facade.
func (m *State) Machine(tag string) (*Machine, error) {
	life, err := m.machineLife(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:    tag,
		life:   life,
		mstate: m,
	}, nil
}
