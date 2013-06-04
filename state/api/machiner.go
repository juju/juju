// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import "launchpad.net/juju-core/state/api/params"

// Machiner provides access to the Machiner API facade.
type Machiner struct {
	st *State
}

// MachinerMachine provides access to state.Machine methods through
// the Machiner facade.
type MachinerMachine struct {
	id       string
	machiner *Machiner
}

// Machine provides access to methods of a state.Machine through the facade.
func (m *Machiner) Machine(id string) (*MachinerMachine, error) {
	var result params.MachinesResults
	args := params.Machines{
		Ids: []string{id},
	}
	err := m.st.call("Machiner", "", "Machines", args, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Machines[0].Error; err != nil {
		return nil, err
	}
	return &MachinerMachine{
		id:       result.Machines[0].Machine.Id,
		machiner: m,
	}, nil
}

// SetStatus changes the status of the machine.
func (mm *MachinerMachine) SetStatus(status params.Status, info string) error {
	var result params.ErrorResults
	args := params.MachinesSetStatus{
		Machines: []params.MachineSetStatus{
			{Id: mm.id, Status: status, Info: info},
		},
	}
	err := mm.machiner.st.call("Machiner", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.Errors[0]
}

// Refresh is a no-op needed for interface compatibility.
func (mm *MachinerMachine) Refresh() error {
	return nil
}

// Id returns the machine id.
func (mm *MachinerMachine) Id() string {
	return mm.id
}

// Life returns the machine's lifecycle state.
func (mm *MachinerMachine) Life() (params.Life, error) {
	var result params.MachinesLifeResults
	args := params.Machines{
		Ids: []string{mm.id},
	}
	err := mm.machiner.st.call("Machiner", "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if err := result.Machines[0].Error; err != nil {
		return "", err
	}
	return result.Machines[0].Life, nil
}
