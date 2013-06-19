// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import "launchpad.net/juju-core/state/api/params"

// Machine represents a juju machine as seen by a machiner worker.
type Machine struct {
	id     string
	life   params.Life
	mstate *State
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status params.Status, info string) error {
	var result params.ErrorResults
	args := params.MachinesSetStatus{
		Machines: []params.MachineSetStatus{
			{Id: m.id, Status: status, Info: info},
		},
	}
	err := m.mstate.caller.Call("Machiner", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.Errors[0]
}

// Refresh updates the cached local copy of the machine's data.
func (m *Machine) Refresh() error {
	life, err := m.mstate.machineLife(m.id)
	if err != nil {
		return err
	}
	m.life = life
	return nil
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (m *Machine) EnsureDead() error {
	var result params.ErrorResults
	args := params.Machines{
		Ids: []string{m.id},
	}
	err := m.mstate.caller.Call("Machiner", "", "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.Errors[0]
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.id
}

// Life returns the machine's lifecycle value.
func (m *Machine) Life() params.Life {
	return m.life
}
