// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineagent
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
// required by the machine agent code.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

func (st *State) getMachine(id string) (*params.MachineAgentGetMachinesResult, error) {
	var results params.MachineAgentGetMachinesResults
	args := params.Machines{
		Ids: []string{id},
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
	st *State
	id string
	doc params.MachineAgentGetMachinesResult
}

func (st *State) Machine(id string) (*Machine, error) {
	doc, err := st.getMachine(id)
	if err != nil {
		return nil, err
	}
	return &Machine{
		st: st,
		id: id,
		doc: *doc,
	}, nil
}

func (m *Machine) Id() string {
	return m.id
}

func (m *Machine) Life() params.Life {
	return m.doc.Life
}

func (m *Machine) Jobs() []params.MachineJob {
	return m.doc.Jobs
}

func (m *Machine) Refresh() error {
	doc, err := m.st.getMachine(m.id)
	if err != nil {
		return err
	}
	m.doc = *doc
	return nil
}
