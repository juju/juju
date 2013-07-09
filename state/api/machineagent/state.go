// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineagent

// TODO(fwereade): there's nothing machine-specific in here...

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// State provides access to a machine agent's view of the state.
type State struct {
	caller common.Caller
}

// Machiner returns a version of the state that provides functionality
// required by the machine agent code.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

func (st *State) getMachine(tag string) (*params.MachineAgentGetMachinesResult, error) {
	var results params.MachineAgentGetMachinesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("MachineAgent", "", "GetMachines", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Machines) != 1 {
		return nil, fmt.Errorf("expected one result, got %d", len(results.Machines))
	}
	if err := results.Machines[0].Error; err != nil {
		return nil, err
	}
	return &results.Machines[0], nil
}

type Machine struct {
	st  *State
	tag string
	doc params.MachineAgentGetMachinesResult
}

func (st *State) Machine(tag string) (*Machine, error) {
	doc, err := st.getMachine(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		st:  st,
		tag: tag,
		doc: *doc,
	}, nil
}

func (m *Machine) Tag() string {
	return m.tag
}

func (m *Machine) Life() params.Life {
	return m.doc.Life
}

func (m *Machine) Jobs() []params.MachineJob {
	return m.doc.Jobs
}

func (m *Machine) SetPassword(password string) error {
	var results params.ErrorResults
	args := params.PasswordChanges{
		Changes: []params.PasswordChange{{
			Tag:      m.tag,
			Password: password,
		}},
	}
	err := m.st.caller.Call("MachineAgent", "", "SetPasswords", args, &results)
	if err != nil {
		return err
	}
	if len(results.Errors) > 0 && results.Errors[0] != nil {
		return results.Errors[0]
	}
	return nil
}
